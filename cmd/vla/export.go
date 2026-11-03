package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/logging"
)

type exportOptions struct {
	sessionsPath string
	since        time.Duration
	outDir       string
	armName      string
	camName      string
	instruction  string
	orgID        string
	locationID   string
	partID       string
	datasetID    string
}

type session struct {
	id    string
	start time.Time
	end   time.Time
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
	if opts.datasetID != "" {
		return exportDataset(ctx, dc, opts, logger)
	}

	sessions, err := readSessions(opts.sessionsPath)
	if err != nil {
		return fmt.Errorf("reading sessions: %w", err)
	}

	if opts.since > 0 {
		cutoff := time.Now().Add(-opts.since)
		filtered := sessions[:0]
		for _, s := range sessions {
			if s.start.After(cutoff) {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	logger.Infof("exporting %d session(s) from %s", len(sessions), opts.sessionsPath)

	if len(sessions) == 0 {
		return nil
	}

	if err := os.MkdirAll(opts.outDir, 0o755); err != nil {
		return err
	}

	scope := scopeFilter{orgID: opts.orgID, locationID: opts.locationID, partID: opts.partID}

	for _, s := range sessions {
		cam, err := fetchBinaryInRange(ctx, dc, s.start, s.end, opts.camName, "GetImages", scope, logger)
		if err != nil {
			return fmt.Errorf("session %s: fetching cam: %w", s.id, err)
		}
		sort.Slice(cam, func(i, j int) bool { return cam[i].t.Before(cam[j].t) })

		arm, err := fetchTabularInRange(ctx, dc, s.start, s.end, opts.armName, "JointPositions", scope, logger)
		if err != nil {
			return fmt.Errorf("session %s: fetching arm: %w", s.id, err)
		}
		sort.Slice(arm, func(i, j int) bool { return arm[i].t.Before(arm[j].t) })

		if err := writeEpisode(opts.outDir, s.id, arm, cam, opts.instruction, logger); err != nil {
			return fmt.Errorf("session %s: %w", s.id, err)
		}
	}
	return nil
}

func exportDataset(ctx context.Context, dc *app.DataClient, opts exportOptions, logger logging.Logger) error {
	logger.Infof("exporting dataset: %s", opts.datasetID)
	if err := os.MkdirAll(opts.outDir, 0o755); err != nil {
		return err
	}

	const pageSize = 50
	page, err := dc.SequencesByDatasetID(ctx, opts.datasetID, pageSize)
	if err != nil {
		return fmt.Errorf("listing sequences for dataset %s: %w", opts.datasetID, err)
	}

	total := 0
	for len(page.Sequences) > 0 {
		for _, seq := range page.Sequences {
			if seq == nil {
				continue
			}
			if err := exportSequence(ctx, dc, opts, seq, logger); err != nil {
				return fmt.Errorf("sequence %s: %w", seq.ID, err)
			}
			total++
		}
		page, err = page.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("paging sequences for dataset %s: %w", opts.datasetID, err)
		}
	}

	logger.Infof("exported %d sequence(s) from dataset %s", total, opts.datasetID)
	return nil
}

func exportSequence(ctx context.Context, dc *app.DataClient, opts exportOptions, seq *app.Sequence, logger logging.Logger) error {
	partID := seq.PartID
	if partID == "" {
		partID = opts.partID
	}
	scope := scopeFilter{orgID: opts.orgID, locationID: opts.locationID, partID: partID}

	logger.Infof("exporting sequence %s part=%s interval=[%s,%s] tags=%v resources=%d",
		seq.ID, seq.PartID,
		seq.StartTime.Format(time.RFC3339), seq.EndTime.Format(time.RFC3339),
		seq.SequenceTags, len(seq.Resources))

	var arm []tabularRow
	var cam []binaryRow
	for _, r := range seq.Resources {
		switch r.MethodName {
		case "GetImages":
			rows, err := fetchBinaryInRange(ctx, dc, seq.StartTime, seq.EndTime, r.ResourceName, r.MethodName, scope, logger)
			if err != nil {
				return fmt.Errorf("fetching binary %s/%s: %w", r.ResourceName, r.MethodName, err)
			}
			cam = append(cam, rows...)
		case "JointPositions":
			rows, err := fetchTabularInRange(ctx, dc, seq.StartTime, seq.EndTime, r.ResourceName, r.MethodName, scope, logger)
			if err != nil {
				return fmt.Errorf("fetching tabular %s/%s: %w", r.ResourceName, r.MethodName, err)
			}
			arm = append(arm, rows...)
		default:
			logger.Warnf("sequence %s: skipping unsupported resource %s/%s", seq.ID, r.ResourceName, r.MethodName)
		}
	}

	sort.Slice(cam, func(i, j int) bool { return cam[i].t.Before(cam[j].t) })
	sort.Slice(arm, func(i, j int) bool { return arm[i].t.Before(arm[j].t) })

	return writeEpisode(opts.outDir, seq.ID, arm, cam, opts.instruction, logger)
}

type scopeFilter struct {
	// orgID is the organization TabularDataByMQL queries; it is not applied to binary filters.
	orgID      string
	locationID string
	partID     string
}

func (s scopeFilter) apply(f *app.Filter) {
	if s.locationID != "" {
		f.LocationIDs = []string{s.locationID}
	}
	if s.partID != "" {
		f.PartID = s.partID
	}
}

func readSessions(path string) ([]session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1

	var out []session
	first := true
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if first {
			first = false
			if len(row) > 0 && row[0] == "session_id" {
				continue
			}
		}
		if len(row) < 3 {
			return nil, fmt.Errorf("malformed sessions row: %v", row)
		}
		start, err := time.Parse(time.RFC3339Nano, row[1])
		if err != nil {
			return nil, fmt.Errorf("parsing start %q: %w", row[1], err)
		}
		end, err := time.Parse(time.RFC3339Nano, row[2])
		if err != nil {
			return nil, fmt.Errorf("parsing end %q: %w", row[2], err)
		}
		out = append(out, session{id: row[0], start: start, end: end})
	}
	return out, nil
}

func fetchBinaryInRange(ctx context.Context, dc *app.DataClient, start, end time.Time, name, method string, scope scopeFilter, logger logging.Logger) ([]binaryRow, error) {
	var out []binaryRow
	last := ""
	pages := 0
	for {
		filter := &app.Filter{
			ComponentName: name,
			Method:        method,
			Interval:      app.CaptureInterval{Start: start, End: end},
		}
		scope.apply(filter)
		options := &app.DataByFilterOptions{Filter: filter, Last: last}
		logger.Infof("BinaryDataByFilter component=%s method=%s interval=[%s,%s] location=%v part=%s last=%q",
			filter.ComponentName,
			filter.Method,
			filter.Interval.Start.Format(time.RFC3339Nano),
			filter.Interval.End.Format(time.RFC3339Nano),
			filter.LocationIDs,
			filter.PartID,
			options.Last,
		)
		resp, err := dc.BinaryDataByFilter(ctx, false, options)
		if err != nil {
			return nil, err
		}
		for _, bd := range resp.BinaryData {
			if bd.Metadata == nil || bd.Metadata.BinaryDataID == "" {
				continue
			}
			ext := bd.Metadata.FileExt
			if ext == "" {
				ext = ".jpg"
			}
			out = append(out, binaryRow{
				id:  bd.Metadata.BinaryDataID,
				t:   bd.Metadata.TimeRequested,
				ext: ext,
			})
		}
		pages++
		if resp.Last == "" || resp.Last == last || len(resp.BinaryData) == 0 {
			break
		}
		last = resp.Last
	}

	if err := hydrateBinary(ctx, dc, out, logger); err != nil {
		return nil, err
	}
	logger.Infof("fetched binary: %s/%s in [%s, %s] — %d pages, %d items", name, method, start.Format(time.RFC3339), end.Format(time.RFC3339), pages, len(out))
	return out, nil
}

func hydrateBinary(ctx context.Context, dc *app.DataClient, rows []binaryRow, logger logging.Logger) error {
	const batch = 2
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

// tabularMatch builds the $match stage for rows of one resource method captured
// in [start, end], scoped to the configured location and part. Empty name,
// method, location, or part and zero bounds are left unconstrained.
func (s scopeFilter) tabularMatch(start, end time.Time, name, method string) map[string]any {
	match := map[string]any{}
	if name != "" {
		match["component_name"] = name
	}
	if method != "" {
		match["method_name"] = method
	}
	if s.locationID != "" {
		match["location_id"] = s.locationID
	}
	if s.partID != "" {
		match["part_id"] = s.partID
	}
	interval := map[string]any{}
	if !start.IsZero() {
		interval["$gte"] = start
	}
	if !end.IsZero() {
		interval["$lte"] = end
	}
	if len(interval) > 0 {
		match["time_requested"] = interval
	}
	return match
}

func fetchTabularInRange(ctx context.Context, dc *app.DataClient, start, end time.Time, name, method string, scope scopeFilter, logger logging.Logger) ([]tabularRow, error) {
	if scope.orgID == "" {
		return nil, errors.New("querying tabular data needs an organization ID")
	}
	match := scope.tabularMatch(start, end, name, method)
	logger.Infof("TabularDataByMQL org=%s component=%s method=%s interval=[%s,%s] location=%s part=%s",
		scope.orgID,
		name,
		method,
		start.Format(time.RFC3339Nano),
		end.Format(time.RFC3339Nano),
		scope.locationID,
		scope.partID,
	)
	docs, err := dc.TabularDataByMQL(ctx, scope.orgID, []map[string]any{
		{"$match": match},
		{"$sort": map[string]any{"time_requested": 1}},
		{"$project": map[string]any{"_id": 0, "time_requested": 1, "data": 1}},
	}, nil)
	if err != nil {
		return nil, err
	}
	out := make([]tabularRow, 0, len(docs))
	for _, doc := range docs {
		t, ok := doc["time_requested"].(time.Time)
		if !ok {
			return nil, fmt.Errorf("tabular row has no time_requested timestamp: %v", doc["time_requested"])
		}
		data, _ := doc["data"].(map[string]any)
		out = append(out, tabularRow{t: t, data: data})
	}
	logger.Infof("fetched tabular: %s/%s in [%s, %s] — %d rows", name, method, start.Format(time.RFC3339), end.Format(time.RFC3339), len(out))
	return out, nil
}

func writeEpisode(outDir, id string, arm []tabularRow, cam []binaryRow, instruction string, logger logging.Logger) error {
	if len(cam) == 0 {
		logger.Warnf("session %s has no images, skipping", id)
		return nil
	}

	epDir := filepath.Join(outDir, id)
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

	logger.Infof("wrote episode %s — %d steps, %d joint samples available", id, len(cam), len(arm))
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
	// BSON decoding yields int for whole numbers stored as integers.
	for i, v := range vals {
		switch n := v.(type) {
		case float64:
			out[i] = n
		case int:
			out[i] = float64(n)
		}
	}
	return out
}
