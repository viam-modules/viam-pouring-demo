package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/logging"
)

type exportOptions struct {
	since       time.Duration
	outDir      string
	tagPrefix   string
	armName     string
	camName     string
	instruction string
}

type tabularRow struct {
	t    time.Time
	data map[string]any
}

type binaryRow struct {
	id   string
	t    time.Time
	data []byte
	ext  string
}

func runExport(ctx context.Context, dc *app.DataClient, opts exportOptions, logger logging.Logger) error {
	end := time.Now()
	start := end.Add(-opts.since)
	logger.Infof("exporting episodes tagged %q* between %s and %s", opts.tagPrefix, start.Format(time.RFC3339), end.Format(time.RFC3339))

	camByTag, err := fetchBinaryByTag(ctx, dc, start, end, opts.tagPrefix, opts.camName, "GetImages", logger)
	if err != nil {
		return fmt.Errorf("fetching cam binary: %w", err)
	}

	if len(camByTag) == 0 {
		logger.Warnf("no episodes found")
		return nil
	}

	if err := os.MkdirAll(opts.outDir, 0o755); err != nil {
		return err
	}

	tags := make([]string, 0, len(camByTag))
	for t := range camByTag {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	for _, tag := range tags {
		cam := camByTag[tag]
		sort.Slice(cam, func(i, j int) bool { return cam[i].t.Before(cam[j].t) })

		// Pad bounds slightly so a joint sample taken just before/after the
		// first/last image is still picked up.
		const pad = 500 * time.Millisecond
		epStart := cam[0].t.Add(-pad)
		epEnd := cam[len(cam)-1].t.Add(pad)

		arm, err := fetchTabularInRange(ctx, dc, epStart, epEnd, opts.armName, "JointPositions", logger)
		if err != nil {
			return fmt.Errorf("episode %s: fetching arm tabular: %w", tag, err)
		}
		sort.Slice(arm, func(i, j int) bool { return arm[i].t.Before(arm[j].t) })

		if err := writeEpisode(opts.outDir, tag, arm, cam, opts.instruction, logger); err != nil {
			return fmt.Errorf("episode %s: %w", tag, err)
		}
	}
	return nil
}

func fetchBinaryByTag(ctx context.Context, dc *app.DataClient, start, end time.Time, tagPrefix, name, method string, logger logging.Logger) (map[string][]binaryRow, error) {
	out := map[string][]binaryRow{}
	last := ""
	pages := 0
	totalMeta := 0
	for {
		resp, err := dc.BinaryDataByFilter(ctx, false, &app.DataByFilterOptions{
			Filter: &app.Filter{
				ComponentName: name,
				Method:        method,
				Interval:      app.CaptureInterval{Start: start, End: end},
				TagsFilter:    app.TagsFilter{Type: app.TagsFilterTypeTagged},
			},
			Last: last,
		})
		if err != nil {
			return nil, err
		}
		for _, bd := range resp.BinaryData {
			if bd.Metadata == nil || bd.Metadata.BinaryDataID == "" {
				continue
			}
			for _, t := range bd.Metadata.CaptureMetadata.Tags {
				if !strings.HasPrefix(t, tagPrefix) {
					continue
				}
				ext := bd.Metadata.FileExt
				if ext == "" {
					ext = ".jpg"
				}
				out[t] = append(out[t], binaryRow{
					id:  bd.Metadata.BinaryDataID,
					t:   bd.Metadata.TimeRequested,
					ext: ext,
				})
				totalMeta++
			}
		}
		pages++
		if resp.Last == "" || resp.Last == last || len(resp.BinaryData) == 0 {
			break
		}
		last = resp.Last
	}
	logger.Infof("fetched binary metadata: %s/%s — %d pages, %d matching tags, %d items", name, method, pages, len(out), totalMeta)

	for tag, rows := range out {
		if err := hydrateBinary(ctx, dc, rows, logger); err != nil {
			return nil, fmt.Errorf("hydrating tag %s: %w", tag, err)
		}
	}
	return out, nil
}

func hydrateBinary(ctx context.Context, dc *app.DataClient, rows []binaryRow, logger logging.Logger) error {
	const batch = 50
	byID := make(map[string]int, len(rows))
	ids := make([]string, 0, len(rows))
	for i, r := range rows {
		byID[r.id] = i
		ids = append(ids, r.id)
	}
	for i := 0; i < len(ids); i += batch {
		end := min(i+batch, len(ids))
		chunk := ids[i:end]
		data, err := dc.BinaryDataByIDs(ctx, chunk)
		if err != nil {
			return err
		}
		for _, bd := range data {
			if bd.Metadata == nil {
				continue
			}
			idx, ok := byID[bd.Metadata.BinaryDataID]
			if !ok {
				continue
			}
			rows[idx].data = bd.Binary
		}
	}
	logger.Debugf("hydrated %d binary rows", len(rows))
	return nil
}

func fetchTabularInRange(ctx context.Context, dc *app.DataClient, start, end time.Time, name, method string, logger logging.Logger) ([]tabularRow, error) {
	var out []tabularRow
	last := ""
	pages := 0
	for {
		resp, err := dc.TabularDataByFilter(ctx, &app.DataByFilterOptions{
			Filter: &app.Filter{
				ComponentName: name,
				Method:        method,
				Interval:      app.CaptureInterval{Start: start, End: end},
			},
			Last: last,
		})
		if err != nil {
			return nil, err
		}
		for _, td := range resp.TabularData {
			out = append(out, tabularRow{t: td.TimeRequested, data: td.Data})
		}
		pages++
		if resp.Last == "" || resp.Last == last || len(resp.TabularData) == 0 {
			break
		}
		last = resp.Last
	}
	logger.Debugf("fetched tabular: %s/%s in [%s, %s] — %d pages, %d rows", name, method, start.Format(time.RFC3339), end.Format(time.RFC3339), pages, len(out))
	return out, nil
}

func writeEpisode(outDir, tag string, arm []tabularRow, cam []binaryRow, instruction string, logger logging.Logger) error {
	if len(cam) == 0 {
		logger.Warnf("episode %s has no images, skipping", tag)
		return nil
	}

	epDir := filepath.Join(outDir, tag)
	imgDir := filepath.Join(epDir, "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(epDir, "steps.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)

	stateAt := func(t time.Time) []float64 {
		row, ok := closestTabular(arm, t)
		if !ok {
			return nil
		}
		return extractJointPositions(row.data)
	}

	for i, c := range cam {
		state := stateAt(c.t)

		var action []float64
		if i+1 < len(cam) {
			action = stateAt(cam[i+1].t)
		} else {
			action = state
		}

		imgRel := filepath.Join("images", fmt.Sprintf("step_%06d%s", i, c.ext))
		if err := os.WriteFile(filepath.Join(epDir, imgRel), c.data, 0o644); err != nil {
			return err
		}

		step := map[string]any{
			"step_index":           i,
			"timestamp":            c.t.UTC().Format(time.RFC3339Nano),
			"is_first":             i == 0,
			"is_last":              i == len(cam)-1,
			"is_terminal":          i == len(cam)-1,
			"image":                imgRel,
			"observation_state":    state,
			"action":               action,
			"language_instruction": instruction,
		}
		if err := enc.Encode(step); err != nil {
			return err
		}
	}

	logger.Infof("wrote episode %s — %d steps, %d joint samples available", tag, len(cam), len(arm))
	return nil
}

func closestTabular(rows []tabularRow, t time.Time) (tabularRow, bool) {
	if len(rows) == 0 {
		return tabularRow{}, false
	}
	best := rows[0]
	bestD := absDur(rows[0].t.Sub(t))
	for _, r := range rows[1:] {
		d := absDur(r.t.Sub(t))
		if d < bestD {
			best = r
			bestD = d
		}
	}
	return best, true
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func extractJointPositions(data map[string]any) []float64 {
	if data == nil {
		return nil
	}
	p, ok := data["positions"].(map[string]any)
	if !ok {
		return nil
	}
	vals, ok := p["values"].([]any)
	if !ok {
		return nil
	}
	out := make([]float64, len(vals))
	for i, v := range vals {
		if f, ok := v.(float64); ok {
			out[i] = f
		}
	}
	return out
}
