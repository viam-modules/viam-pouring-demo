// Command cup-heatmap sweeps cup-placement positions across the wine cart
// surface and uses armplanning.PlanMotion (no arm movement) to produce a
// black/white PNG heatmap of pickable vs. dead zones.
//
// The cart surface is pulled dynamically from the `obstacle-table` frame in
// the live frame system, so retuning the cart in the robot config and
// re-running the script picks up the new bounds with no code change.
//
// Usage:
//
//	bin/cup-heatmap \
//	  --host vino1-main.kssbd6djf3.viam.cloud \
//	  --config service.json \
//	  --step-mm 10 \
//	  --out cup-heatmap.png \
//	  --details-out cup-heatmap.json
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang/geo/r3"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot/framesystem"
	"go.viam.com/rdk/spatialmath"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

// cartFrameName is the frame in the live frame system whose box geometry
// defines the cup-placement surface (X/Y bounds and top-Z). Changing the
// surface means editing the robot config, not this constant.
const cartFrameName = "obstacle-table"

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintf(os.Stderr, "cup-heatmap: %v\n", err)
		os.Exit(1)
	}
}

func realMain() error {
	host := flag.String("host", "", "machine FQDN (required)")
	configFile := flag.String("config", "", "pour service JSON config file (required)")
	stepMM := flag.Float64("step-mm", 10, "grid step in mm")
	outPNG := flag.String("out", "cup-heatmap.png", "output PNG path")
	detailsOut := flag.String("details-out", "cup-heatmap.json", "output sidecar JSON path")
	workers := flag.Int("workers", 4, "concurrent plan workers")
	startJointsFromConfig := flag.String("start-joints-from-config", "",
		"comma-separated arm-position-saver switch names to read start joints from "+
			"(default: read live joints from each arm)")
	resume := flag.Bool("resume", false,
		"resume from an existing --details-out sidecar: keep cells that already have "+
			"a non-error result and only re-plan the previously errored ones")
	renderOnly := flag.Bool("render-only", false,
		"do not connect or plan; just re-render --out PNG from the existing --details-out sidecar")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	if *stepMM <= 0 {
		return fmt.Errorf("--step-mm must be > 0")
	}
	if *workers <= 0 {
		return fmt.Errorf("--workers must be > 0")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := logging.NewLogger("cup-heatmap")
	if *debug {
		logger.SetLevel(logging.DEBUG)
	} else {
		logger.SetLevel(logging.INFO)
	}

	if *renderOnly {
		return renderOnlyFromSidecar(*detailsOut, *outPNG, logger)
	}

	if *host == "" {
		return fmt.Errorf("--host is required")
	}
	if *configFile == "" {
		return fmt.Errorf("--config is required")
	}

	cfg := &pour.Config{}
	if err := vmodutils.ReadJSONFromFile(*configFile, cfg); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}
	if _, _, err := cfg.Validate(""); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	logger.Infof("connecting to %s", *host)
	client, err := vmodutils.ConnectToHostFromCLIToken(ctx, *host, logger)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close(ctx)

	deps, err := vmodutils.MachineToDependencies(client)
	if err != nil {
		return fmt.Errorf("deps: %w", err)
	}

	p1c, err := pour.Pour1ComponentsFromDependencies(cfg, deps)
	if err != nil {
		return fmt.Errorf("pour components: %w", err)
	}

	vc, err := pour.NewVinoCart(ctx, cfg, p1c, client, nil, logger)
	if err != nil {
		return fmt.Errorf("VinoCart: %w", err)
	}

	bounds, err := resolveCartBounds(ctx, p1c.Rfs, cartFrameName)
	if err != nil {
		return fmt.Errorf("cart bounds: %w", err)
	}
	cupCenterZ := bounds.TopZ + cfg.CupHeight/2
	logger.Infof("cart bounds: X=[%.1f, %.1f] Y=[%.1f, %.1f] top_z=%.1f step=%.1fmm",
		bounds.XMin, bounds.XMax, bounds.YMin, bounds.YMax, bounds.TopZ, *stepMM)
	logger.Infof("cup geometry: width=%.1fmm height=%.1fmm center_z=%.1fmm",
		cfg.CupWidth, cfg.CupHeight, cupCenterZ)

	startInputs, err := resolveStartInputs(ctx, deps, cfg, *startJointsFromConfig, logger)
	if err != nil {
		return fmt.Errorf("start inputs: %w", err)
	}

	// Build the frame system once. The per-cell PlanPickAt hot path is then RPC-free,
	// so a transient disconnect to the machine no longer turns every remaining cell
	// into an error (which is what produced the all-grey "corrupt" PNGs).
	fs, err := vc.BuildPickFrameSystem(ctx)
	if err != nil {
		return fmt.Errorf("build frame system: %w", err)
	}

	// -- grid setup --

	nx := int(math.Ceil((bounds.XMax-bounds.XMin)/(*stepMM))) + 1
	ny := int(math.Ceil((bounds.YMax-bounds.YMin)/(*stepMM))) + 1
	total := nx * ny
	logger.Infof("grid: %d x %d = %d cells; %d workers", nx, ny, total, *workers)

	sidecar := &Sidecar{
		Bounds: SidecarBounds{
			XMin:       bounds.XMin,
			XMax:       bounds.XMax,
			YMin:       bounds.YMin,
			YMax:       bounds.YMax,
			TopZ:       bounds.TopZ,
			StepMM:     *stepMM,
			CartFrame:  cartFrameName,
			GridWidth:  nx,
			GridHeight: ny,
		},
		Cup: SidecarCup{
			WidthMM:    cfg.CupWidth,
			HeightMM:   cfg.CupHeight,
			CenterZmm:  cupCenterZ,
			GripperZmm: cfg.CupHeight - cupGripHeightOffsetFromCfg(cfg),
		},
	}

	cells := make([]*Cell, total)

	// Resume: preload non-error cells from a previous run's sidecar so we only
	// re-plan the ones that errored (e.g. due to a mid-run disconnect).
	preloaded := 0
	if *resume {
		n, rerr := preloadResume(*detailsOut, sidecar, cells, nx, ny)
		if rerr != nil {
			return fmt.Errorf("resume from %s: %w", *detailsOut, rerr)
		}
		preloaded = n
		logger.Infof("resume: preloaded %d / %d cells from %s; %d remain",
			preloaded, total, *detailsOut, total-preloaded)
	}

	// Watch SIGINT so we can flush partial results before exit.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Warn("interrupt received, draining workers and flushing partial results")
		cancel()
	}()

	// -- worker pool --

	type job struct{ ix, iy int }
	type resultMsg struct {
		idx  int
		cell *Cell
	}

	jobs := make(chan job, *workers*2)
	results := make(chan resultMsg, *workers*2)

	var wg sync.WaitGroup
	wg.Add(*workers)
	for w := 0; w < *workers; w++ {
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				cx := bounds.XMin + float64(j.ix)*(*stepMM)
				cy := bounds.YMin + float64(j.iy)*(*stepMM)
				center := r3.Vector{X: cx, Y: cy, Z: cupCenterZ}
				cellStart := time.Now()
				res, planErr := vc.PlanPickAt(ctx, fs, center, startInputs)
				elapsed := time.Since(cellStart)
				cell := cellFromResult(j.ix, j.iy, cx, cy, res, planErr, elapsed)
				results <- resultMsg{idx: j.iy*nx + j.ix, cell: cell}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for iy := 0; iy < ny; iy++ {
			for ix := 0; ix < nx; ix++ {
				// Skip cells preloaded from a previous run (resume mode).
				if cells[iy*nx+ix] != nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case jobs <- job{ix: ix, iy: iy}:
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	// -- collect + incremental flush --

	done := int64(preloaded)
	startWall := time.Now()
	lastFlush := time.Now()
	const flushInterval = 30 * time.Second

	for r := range results {
		cells[r.idx] = r.cell
		n := atomic.AddInt64(&done, 1)
		newDone := n - int64(preloaded)

		if newDone%100 == 0 || n == int64(total) {
			elapsed := time.Since(startWall)
			rate := float64(newDone) / elapsed.Seconds()
			remaining := time.Duration(float64(int64(total)-n)/rate) * time.Second
			logger.Infof("progress: %d / %d (%.1f cells/s, eta %s)",
				n, total, rate, remaining.Truncate(time.Second))
		}

		if time.Since(lastFlush) >= flushInterval || n == int64(total) {
			if ferr := flushSidecar(sidecar, cells, *detailsOut); ferr != nil {
				logger.Warnf("sidecar flush failed: %v", ferr)
			} else {
				logger.Debugf("flushed sidecar (%d cells)", n)
			}
			lastFlush = time.Now()
		}
	}

	if ctx.Err() != nil {
		logger.Warnf("context done, partial run: %v", ctx.Err())
	}

	// -- final outputs --

	if err := flushSidecar(sidecar, cells, *detailsOut); err != nil {
		return fmt.Errorf("write sidecar: %w", err)
	}
	logger.Infof("wrote sidecar JSON: %s", *detailsOut)

	if err := renderPNG(cells, bounds, *stepMM, *outPNG); err != nil {
		return fmt.Errorf("write PNG: %w", err)
	}
	logger.Infof("wrote heatmap PNG: %s", *outPNG)

	// Close vc explicitly so the embedded web server shuts down; ignore any
	// error since we already own client lifecycle via defer.
	_ = vc.Close(ctx)
	return nil
}

// Bounds describes the cart surface in world frame.
type Bounds struct {
	XMin, XMax float64
	YMin, YMax float64
	TopZ       float64
}

// resolveCartBounds finds the named frame in the live frame system, treats
// its (first) box geometry as the cart top, and returns world-frame X/Y
// bounds plus the geometry's top Z.
func resolveCartBounds(ctx context.Context, rfs framesystem.Service, name string) (*Bounds, error) {
	fsCfg, err := rfs.FrameSystemConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("framesystem config: %w", err)
	}
	var part *referenceframe.FrameSystemPart
	for _, p := range fsCfg.Parts {
		if p.FrameConfig == nil {
			continue
		}
		if p.FrameConfig.Name() == name {
			part = p
			break
		}
	}
	if part == nil {
		return nil, fmt.Errorf("frame %q not found in frame system", name)
	}
	if part.FrameConfig.Parent() != referenceframe.World {
		return nil, fmt.Errorf("frame %q parent is %q, expected %q (need a world-relative box)",
			name, part.FrameConfig.Parent(), referenceframe.World)
	}
	if part.ModelFrame == nil {
		return nil, fmt.Errorf("frame %q has no model frame (no attached geometry)", name)
	}
	geoms, err := part.ModelFrame.Geometries([]referenceframe.Input{})
	if err != nil {
		return nil, fmt.Errorf("frame %q geometries: %w", name, err)
	}
	if geoms == nil || len(geoms.Geometries()) == 0 {
		return nil, fmt.Errorf("frame %q has no geometries", name)
	}
	g := geoms.Geometries()[0]
	cfg, err := spatialmath.NewGeometryConfig(g)
	if err != nil {
		return nil, fmt.Errorf("read geometry config: %w", err)
	}
	if cfg.Type != spatialmath.BoxType {
		return nil, fmt.Errorf("frame %q geometry type is %q, expected %q", name, cfg.Type, spatialmath.BoxType)
	}

	center := part.FrameConfig.Pose().Point()
	hx := cfg.X / 2
	hy := cfg.Y / 2
	hz := cfg.Z / 2
	return &Bounds{
		XMin: center.X - hx,
		XMax: center.X + hx,
		YMin: center.Y - hy,
		YMax: center.Y + hy,
		TopZ: center.Z + hz,
	}, nil
}

// Cell is the per-(grid-x, grid-y) record in the sidecar JSON.
type Cell struct {
	IX        int                    `json:"ix"`
	IY        int                    `json:"iy"`
	X         float64                `json:"x"`
	Y         float64                `json:"y"`
	Pickable  bool                   `json:"pickable"`
	Attempts  []pour.PickPlanAttempt `json:"attempts,omitempty"`
	ElapsedMS int64                  `json:"elapsed_ms"`
	Error     string                 `json:"error,omitempty"`
}

// Sidecar is the on-disk JSON shape: bounds + cup metadata + per-cell results.
type Sidecar struct {
	Bounds SidecarBounds `json:"bounds"`
	Cup    SidecarCup    `json:"cup"`
	Cells  []*Cell       `json:"cells"`
}

// SidecarBounds documents the surface region and grid resolution.
type SidecarBounds struct {
	XMin       float64 `json:"x_min"`
	XMax       float64 `json:"x_max"`
	YMin       float64 `json:"y_min"`
	YMax       float64 `json:"y_max"`
	TopZ       float64 `json:"top_z"`
	StepMM     float64 `json:"step_mm"`
	CartFrame  string  `json:"cart_frame"`
	GridWidth  int     `json:"grid_width"`
	GridHeight int     `json:"grid_height"`
}

// SidecarCup documents the synthetic cup used in planning.
type SidecarCup struct {
	WidthMM    float64 `json:"width_mm"`
	HeightMM   float64 `json:"height_mm"`
	CenterZmm  float64 `json:"center_z_mm"`
	GripperZmm float64 `json:"gripper_z_mm"`
}

func cellFromResult(ix, iy int, x, y float64, res *pour.PickPlanResult, err error, elapsed time.Duration) *Cell {
	c := &Cell{IX: ix, IY: iy, X: x, Y: y, ElapsedMS: elapsed.Milliseconds()}
	if err != nil {
		c.Error = err.Error()
		return c
	}
	if res != nil {
		c.Pickable = res.Pickable
		c.Attempts = res.Attempts
	}
	return c
}

// renderOnlyFromSidecar reads an existing sidecar JSON and re-renders the PNG
// without connecting to the machine or replanning. Useful for iterating on
// annotation/styling changes against an already-computed sweep.
func renderOnlyFromSidecar(detailsPath, outPath string, logger logging.Logger) error {
	f, err := os.Open(detailsPath)
	if err != nil {
		return fmt.Errorf("open sidecar: %w", err)
	}
	defer f.Close()
	var prev Sidecar
	if err := json.NewDecoder(f).Decode(&prev); err != nil {
		return fmt.Errorf("decode sidecar: %w", err)
	}
	bounds := &Bounds{
		XMin: prev.Bounds.XMin,
		XMax: prev.Bounds.XMax,
		YMin: prev.Bounds.YMin,
		YMax: prev.Bounds.YMax,
		TopZ: prev.Bounds.TopZ,
	}
	if err := renderPNG(prev.Cells, bounds, prev.Bounds.StepMM, outPath); err != nil {
		return fmt.Errorf("render PNG: %w", err)
	}
	logger.Infof("wrote heatmap PNG: %s (%d cells from %s)", outPath, len(prev.Cells), detailsPath)
	return nil
}

// preloadResume reads an existing sidecar JSON at path, validates that its
// grid/bounds/cup metadata match the current run's settings, and copies every
// cell that completed without error into cells[]. Errored cells are left nil
// so the worker pool re-plans them. Returns the number of cells preloaded.
func preloadResume(path string, current *Sidecar, cells []*Cell, nx, ny int) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var prev Sidecar
	dec := json.NewDecoder(f)
	if err := dec.Decode(&prev); err != nil {
		return 0, fmt.Errorf("decode sidecar: %w", err)
	}

	if err := compareBounds(prev.Bounds, current.Bounds); err != nil {
		return 0, err
	}
	if err := compareCup(prev.Cup, current.Cup); err != nil {
		return 0, err
	}

	n := 0
	for _, c := range prev.Cells {
		if c == nil {
			continue
		}
		if c.Error != "" {
			// previously errored — let the worker pool re-plan it
			continue
		}
		if c.IX < 0 || c.IX >= nx || c.IY < 0 || c.IY >= ny {
			continue
		}
		idx := c.IY*nx + c.IX
		if cells[idx] != nil {
			continue
		}
		cells[idx] = c
		n++
	}
	return n, nil
}

func compareBounds(prev, cur SidecarBounds) error {
	if prev.GridWidth != cur.GridWidth || prev.GridHeight != cur.GridHeight {
		return fmt.Errorf("grid size mismatch (resume %dx%d vs current %dx%d)",
			prev.GridWidth, prev.GridHeight, cur.GridWidth, cur.GridHeight)
	}
	if prev.CartFrame != cur.CartFrame {
		return fmt.Errorf("cart frame mismatch (resume %q vs current %q)",
			prev.CartFrame, cur.CartFrame)
	}
	for _, p := range []struct {
		name     string
		a, b     float64
		tolerMM  float64
	}{
		{"x_min", prev.XMin, cur.XMin, 0.01},
		{"x_max", prev.XMax, cur.XMax, 0.01},
		{"y_min", prev.YMin, cur.YMin, 0.01},
		{"y_max", prev.YMax, cur.YMax, 0.01},
		{"top_z", prev.TopZ, cur.TopZ, 0.01},
		{"step_mm", prev.StepMM, cur.StepMM, 1e-6},
	} {
		if math.Abs(p.a-p.b) > p.tolerMM {
			return fmt.Errorf("bounds.%s mismatch (resume %.4f vs current %.4f)", p.name, p.a, p.b)
		}
	}
	return nil
}

func compareCup(prev, cur SidecarCup) error {
	for _, p := range []struct {
		name string
		a, b float64
	}{
		{"width_mm", prev.WidthMM, cur.WidthMM},
		{"height_mm", prev.HeightMM, cur.HeightMM},
		{"center_z_mm", prev.CenterZmm, cur.CenterZmm},
		{"gripper_z_mm", prev.GripperZmm, cur.GripperZmm},
	} {
		if math.Abs(p.a-p.b) > 0.01 {
			return fmt.Errorf("cup.%s mismatch (resume %.4f vs current %.4f)", p.name, p.a, p.b)
		}
	}
	return nil
}

func flushSidecar(s *Sidecar, cells []*Cell, path string) error {
	s.Cells = s.Cells[:0]
	for _, c := range cells {
		if c == nil {
			continue
		}
		s.Cells = append(s.Cells, c)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func renderPNG(cells []*Cell, bounds *Bounds, stepMM float64, path string) error {
	width := int(math.Ceil(bounds.XMax - bounds.XMin))
	height := int(math.Ceil(bounds.YMax - bounds.YMin))
	if width <= 0 || height <= 0 {
		return fmt.Errorf("non-positive image dims (%dx%d)", width, height)
	}

	gridW := 0
	gridH := 0
	for _, c := range cells {
		if c == nil {
			continue
		}
		if c.IX+1 > gridW {
			gridW = c.IX + 1
		}
		if c.IY+1 > gridH {
			gridH = c.IY + 1
		}
	}

	if gridW == 0 || gridH == 0 {
		return fmt.Errorf("no cells to render")
	}

	pickable := make([][]bool, gridH)
	known := make([][]bool, gridH)
	for i := range pickable {
		pickable[i] = make([]bool, gridW)
		known[i] = make([]bool, gridW)
	}
	for _, c := range cells {
		if c == nil {
			continue
		}
		if c.IY < gridH && c.IX < gridW {
			pickable[c.IY][c.IX] = c.Pickable
			known[c.IY][c.IX] = c.Error == ""
		}
	}

	// RGBA (not Gray) so we can paint a red world-coordinate grid and a black
	// scale bar over the heatmap without losing the underlying data colours.
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	deadColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	pickColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	unknownColor := color.RGBA{R: 96, G: 96, B: 96, A: 255}

	for py := 0; py < height; py++ {
		worldY := bounds.YMin + float64(py)
		iy := int(math.Round((worldY - bounds.YMin) / stepMM))
		if iy < 0 {
			iy = 0
		}
		if iy >= gridH {
			iy = gridH - 1
		}
		for px := 0; px < width; px++ {
			worldX := bounds.XMin + float64(px)
			ix := int(math.Round((worldX - bounds.XMin) / stepMM))
			if ix < 0 {
				ix = 0
			}
			if ix >= gridW {
				ix = gridW - 1
			}
			switch {
			case !known[iy][ix]:
				img.SetRGBA(px, py, unknownColor)
			case pickable[iy][ix]:
				img.SetRGBA(px, py, pickColor)
			default:
				img.SetRGBA(px, py, deadColor)
			}
		}
	}

	drawAnnotations(img, bounds)

	// Encode to a buffer first so we can splice in a pHYs chunk recording the
	// real physical resolution (1 px = 1 mm). With that chunk present most
	// viewers/print dialogs offer an "actual size" option that prints at 1:1
	// instead of the implicit 96-DPI default that shrinks ~28"x41" of cart
	// down to a single letter page.
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	final, err := injectPhysChunk(buf.Bytes(), 1000, 1000)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, final, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// drawAnnotations overlays a 100-mm world-coordinate grid (red), labels every
// 500 mm with world X/Y, and a labelled 200-mm scale bar in the bottom-left.
// All chosen so a printed page is interpretable whether at 1:1 (with pHYs) or
// fit-to-page.
func drawAnnotations(img *image.RGBA, bounds *Bounds) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	minor := color.RGBA{R: 255, G: 110, B: 110, A: 255} // light red, every 100 mm
	major := color.RGBA{R: 255, G: 0, B: 0, A: 255}     // bright red, every 500 mm
	textColor := color.RGBA{R: 200, G: 0, B: 0, A: 255}

	drawVLine := func(x int, c color.RGBA) {
		if x < 0 || x >= width {
			return
		}
		for y := 0; y < height; y++ {
			img.SetRGBA(x, y, c)
		}
	}
	drawHLine := func(y int, c color.RGBA) {
		if y < 0 || y >= height {
			return
		}
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}

	startX := math.Ceil(bounds.XMin/100) * 100
	for wx := startX; wx <= bounds.XMax+0.5; wx += 100 {
		px := int(math.Round(wx - bounds.XMin))
		if isMultipleOf(wx, 500) {
			drawVLine(px-1, major)
			drawVLine(px, major)
		} else {
			drawVLine(px, minor)
		}
	}

	startY := math.Ceil(bounds.YMin/100) * 100
	for wy := startY; wy <= bounds.YMax+0.5; wy += 100 {
		py := int(math.Round(wy - bounds.YMin))
		if isMultipleOf(wy, 500) {
			drawHLine(py-1, major)
			drawHLine(py, major)
		} else {
			drawHLine(py, minor)
		}
	}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(textColor),
		Face: basicfont.Face7x13,
	}
	for wx := startX; wx <= bounds.XMax+0.5; wx += 500 {
		if !isMultipleOf(wx, 500) {
			continue
		}
		px := int(math.Round(wx - bounds.XMin))
		label := fmt.Sprintf("x=%d", int(math.Round(wx)))
		drawTextBackground(img, label, px+3, 11)
		drawer.Dot = fixed.P(px+3, 11)
		drawer.DrawString(label)
	}
	for wy := startY; wy <= bounds.YMax+0.5; wy += 500 {
		if !isMultipleOf(wy, 500) {
			continue
		}
		py := int(math.Round(wy - bounds.YMin))
		label := fmt.Sprintf("y=%d", int(math.Round(wy)))
		drawTextBackground(img, label, 3, py+13)
		drawer.Dot = fixed.P(3, py+13)
		drawer.DrawString(label)
	}

	drawScaleBar(img, 20, height-30)
}

// drawScaleBar draws a 200-mm scale bar with 50-mm and 100-mm tick marks at
// (x, y) (top-left of the bar), with a "200 mm" label above. Sits on a white
// background rect so it's legible over any heatmap colour.
func drawScaleBar(img *image.RGBA, x, y int) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	const length = 200 // mm == pixels at 1 mm/px
	barColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	bg := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	bgRect := clipRect(image.Rect(x-4, y-22, x+length+24, y+8), width, height)
	draw.Draw(img, bgRect, &image.Uniform{C: bg}, image.Point{}, draw.Src)

	for px := x; px <= x+length; px++ {
		for py := y; py <= y+2; py++ {
			setSafe(img, px, py, barColor)
		}
	}
	for i := 0; i <= length; i += 50 {
		h := 6
		if i%100 == 0 {
			h = 10
		}
		for py := y - h; py <= y; py++ {
			setSafe(img, x+i, py, barColor)
		}
	}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(barColor),
		Face: basicfont.Face7x13,
	}
	drawer.Dot = fixed.P(x, y-12)
	drawer.DrawString("200 mm")
}

// drawTextBackground paints a white rectangle behind a label (sized for
// basicfont.Face7x13: 7 px per char, ascent 11, descent 2) so red labels are
// legible over black, white, and grey heatmap regions alike.
func drawTextBackground(img *image.RGBA, label string, x, baseY int) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	w := len(label) * 7
	bg := clipRect(image.Rect(x-1, baseY-11, x+w+1, baseY+3), width, height)
	draw.Draw(img, bg, &image.Uniform{C: color.RGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
}

func setSafe(img *image.RGBA, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return
	}
	img.SetRGBA(x, y, c)
}

func clipRect(r image.Rectangle, width, height int) image.Rectangle {
	if r.Min.X < 0 {
		r.Min.X = 0
	}
	if r.Min.Y < 0 {
		r.Min.Y = 0
	}
	if r.Max.X > width {
		r.Max.X = width
	}
	if r.Max.Y > height {
		r.Max.Y = height
	}
	return r
}

// isMultipleOf treats anything within 0.5 mm of an integer multiple as "on
// grid"; the grid step is generated by floating-point math from XMin/YMin so
// strict math.Mod equality would miss most major lines.
func isMultipleOf(v, step float64) bool {
	r := math.Mod(math.Abs(v), step)
	return r < 0.5 || r > step-0.5
}

// injectPhysChunk splices a pHYs chunk (physical pixel dimensions) into a PNG
// byte stream immediately after the IHDR chunk. pxPerMeter values are the
// physical resolution; for the heatmap we pass 1000 px/m on both axes so the
// declared scale is 1 mm per pixel.
//
// PNG chunk layout: 4-byte big-endian length, 4-byte type, length bytes of
// data, 4-byte CRC32 over (type+data). pHYs data is 4 bytes ppuX + 4 bytes
// ppuY + 1 byte unit (1 == meter).
func injectPhysChunk(pngBytes []byte, pxPerMeterX, pxPerMeterY uint32) ([]byte, error) {
	// Signature (8) + IHDR chunk header (8) + IHDR data (13) + IHDR CRC (4) = 33.
	const ihdrEnd = 33
	if len(pngBytes) < ihdrEnd {
		return nil, fmt.Errorf("png too short (%d bytes)", len(pngBytes))
	}
	if string(pngBytes[12:16]) != "IHDR" {
		return nil, fmt.Errorf("expected IHDR at offset 12, got %q", pngBytes[12:16])
	}

	data := make([]byte, 9)
	binary.BigEndian.PutUint32(data[0:4], pxPerMeterX)
	binary.BigEndian.PutUint32(data[4:8], pxPerMeterY)
	data[8] = 1 // unit = meter

	typeAndData := append([]byte("pHYs"), data...)
	crc := crc32.ChecksumIEEE(typeAndData)

	var chunk bytes.Buffer
	_ = binary.Write(&chunk, binary.BigEndian, uint32(len(data)))
	chunk.Write(typeAndData)
	_ = binary.Write(&chunk, binary.BigEndian, crc)

	out := make([]byte, 0, len(pngBytes)+chunk.Len())
	out = append(out, pngBytes[:ihdrEnd]...)
	out = append(out, chunk.Bytes()...)
	out = append(out, pngBytes[ihdrEnd:]...)
	return out, nil
}

// cupGripHeightOffsetFromCfg mirrors the package-private cupGripHeightOffset()
// so sidecar metadata reports the value the planner actually used.
func cupGripHeightOffsetFromCfg(cfg *pour.Config) float64 {
	if cfg.CupGripHeightOffset > 0 {
		return cfg.CupGripHeightOffset
	}
	return 25
}

// readStartJointsFromSaver fetches the saved joint configuration from an
// erh:vmodutils:arm-position-saver switch by calling DoCommand({"cfg": true}).
// Returns the underlying arm resource name and the joints.
func readStartJointsFromSaver(ctx context.Context, deps resource.Dependencies, saverName string) (string, []referenceframe.Input, error) {
	res, ok := vmodutils.FindDep(deps, saverName)
	if !ok {
		return "", nil, fmt.Errorf("could not find position saver %q", saverName)
	}
	out, err := res.DoCommand(ctx, map[string]interface{}{"cfg": true})
	if err != nil {
		return "", nil, fmt.Errorf("DoCommand cfg on %q: %w", saverName, err)
	}
	raw, ok := out["as_json"].(string)
	if !ok {
		return "", nil, fmt.Errorf("position saver %q returned no as_json", saverName)
	}
	var cfg struct {
		Arm    string    `json:"arm"`
		Joints []float64 `json:"joints"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return "", nil, fmt.Errorf("decode position saver %q cfg: %w", saverName, err)
	}
	if cfg.Arm == "" {
		return "", nil, fmt.Errorf("position saver %q missing arm", saverName)
	}
	if len(cfg.Joints) == 0 {
		return "", nil, fmt.Errorf("position saver %q missing joints", saverName)
	}
	return cfg.Arm, floatsToInputs(cfg.Joints), nil
}

func floatsToInputs(fs []float64) []referenceframe.Input {
	out := make([]referenceframe.Input, len(fs))
	for i, f := range fs {
		out[i] = referenceframe.Input(f)
	}
	return out
}

// resolveStartInputs returns a frame-system-inputs map for every actuated arm
// referenced by the pour config. If startJointsFromConfig is non-empty it is
// parsed as a comma-separated list of arm-position-saver switch names and
// each is dereferenced; any arms not covered by the savers fall back to live
// joint reads.
func resolveStartInputs(
	ctx context.Context,
	deps resource.Dependencies,
	cfg *pour.Config,
	startJointsFromConfig string,
	logger logging.Logger,
) (referenceframe.FrameSystemInputs, error) {
	out := referenceframe.FrameSystemInputs{}

	if startJointsFromConfig != "" {
		savers := strings.Split(startJointsFromConfig, ",")
		for _, s := range savers {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			armName, inputs, err := readStartJointsFromSaver(ctx, deps, s)
			if err != nil {
				return nil, err
			}
			logger.Infof("start joints for %q from saver %q: %v", armName, s, inputs)
			out[armName] = inputs
		}
	}

	wantArms := []string{cfg.ArmName}
	if cfg.BottleArm != "" {
		wantArms = append(wantArms, cfg.BottleArm)
	}

	for _, name := range wantArms {
		if _, ok := out[name]; ok {
			continue
		}
		armRes, ok := vmodutils.FindDep(deps, name)
		if !ok {
			return nil, fmt.Errorf("arm %q not found in deps", name)
		}
		ie, ok := armRes.(interface {
			JointPositions(ctx context.Context, extra map[string]interface{}) ([]referenceframe.Input, error)
		})
		if !ok {
			return nil, fmt.Errorf("arm %q does not expose JointPositions", name)
		}
		joints, err := ie.JointPositions(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("JointPositions on %q: %w", name, err)
		}
		logger.Infof("start joints for %q from live arm: %v", name, joints)
		out[name] = joints
	}

	return out, nil
}
