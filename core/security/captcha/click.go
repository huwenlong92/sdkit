package captcha

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"time"

	"github.com/huwenlong92/sdkit/core/security/captcha/store"
	"github.com/huwenlong92/sdkit/pkg/security/token"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	defaultClickWidth       = 320
	defaultClickHeight      = 160
	defaultClickTargets     = 3
	defaultClickTolerance   = 12
	defaultClickMaxAttempts = 3
)

var defaultClickLabels = []string{"A", "B", "C", "D", "E", "F", "2", "3", "5", "7", "8", "9"}

type ClickOption func(*clickOptions)

type clickOptions struct {
	width       int
	height      int
	targets     int
	tolerance   int
	maxAttempts int
	source      BackgroundSource
}

type ClickProvider struct {
	store store.Store
	ttl   time.Duration
	opts  clickOptions
}

type clickState struct {
	Points      []point   `json:"points"`
	Labels      []string  `json:"labels"`
	Tolerance   int       `json:"tolerance"`
	MaxAttempts int       `json:"max_attempts"`
	Attempts    int       `json:"attempts"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func NewClickProvider(store store.Store, ttl time.Duration, opts ...ClickOption) *ClickProvider {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	options := clickOptions{
		width:       defaultClickWidth,
		height:      defaultClickHeight,
		targets:     defaultClickTargets,
		tolerance:   defaultClickTolerance,
		maxAttempts: defaultClickMaxAttempts,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.width <= 0 {
		options.width = defaultClickWidth
	}
	if options.height <= 0 {
		options.height = defaultClickHeight
	}
	if options.targets <= 0 {
		options.targets = defaultClickTargets
	}
	if options.tolerance < 0 {
		options.tolerance = defaultClickTolerance
	}
	if options.maxAttempts <= 0 {
		options.maxAttempts = defaultClickMaxAttempts
	}
	return &ClickProvider{store: store, ttl: ttl, opts: options}
}

func WithClickSize(width int, height int) ClickOption {
	return func(o *clickOptions) {
		o.width = width
		o.height = height
	}
}

func WithClickTargets(targets int) ClickOption {
	return func(o *clickOptions) {
		o.targets = targets
	}
}

func WithClickTolerance(tolerance int) ClickOption {
	return func(o *clickOptions) {
		o.tolerance = tolerance
	}
}

func WithClickMaxAttempts(maxAttempts int) ClickOption {
	return func(o *clickOptions) {
		o.maxAttempts = maxAttempts
	}
}

func WithClickBackgroundSource(source BackgroundSource) ClickOption {
	return func(o *clickOptions) {
		o.source = source
	}
}

func (p *ClickProvider) Name() string { return "click" }

func (p *ClickProvider) Kind() Kind { return KindClick }

func (p *ClickProvider) Generate(ctx context.Context, opts GenerateOptions) (*Challenge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.store == nil {
		return nil, ErrInvalidToken
	}
	ttl := p.ttl
	if opts.TTL > 0 {
		ttl = opts.TTL
	}
	bg, err := pickBackground(ctx, p.opts.source, p.opts.width, p.opts.height)
	if err != nil {
		return nil, err
	}
	count := minInt(p.opts.targets, len(defaultClickLabels))
	labelPool := append([]string(nil), defaultClickLabels...)
	labels := make([]string, 0, count)
	points := make([]point, 0, count)
	for i := 0; i < count; i++ {
		idx := randomInt(len(labelPool))
		label := labelPool[idx]
		labelPool = append(labelPool[:idx], labelPool[idx+1:]...)
		pt := randomClickPoint(points, p.opts.width, p.opts.height, p.opts.tolerance)
		labels = append(labels, label)
		points = append(points, pt)
		drawClickTarget(bg, pt, label)
	}
	imageData, err := encodePNGDataURL(bg)
	if err != nil {
		return nil, err
	}
	id, err := token.RandomToken(16)
	if err != nil {
		return nil, err
	}
	state := clickState{
		Points:      points,
		Labels:      labels,
		Tolerance:   p.opts.tolerance,
		MaxAttempts: p.opts.maxAttempts,
		ExpiresAt:   time.Now().Add(ttl),
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	if err := p.store.Set(ctx, id, string(raw), ttl); err != nil {
		return nil, err
	}
	return &Challenge{
		ID:       id,
		Kind:     KindClick,
		Image:    imageData,
		ExpireAt: state.ExpiresAt,
		Payload: map[string]any{
			"width":     p.opts.width,
			"height":    p.opts.height,
			"targets":   labels,
			"tolerance": p.opts.tolerance,
		},
	}, nil
}

func (p *ClickProvider) Verify(ctx context.Context, id string, answer string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || p.store == nil {
		return ErrInvalidToken
	}
	raw, ok, err := p.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidToken
	}
	var state clickState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		_ = p.store.Delete(ctx, id)
		return ErrInvalidToken
	}
	if time.Now().After(state.ExpiresAt) {
		_ = p.store.Delete(ctx, id)
		return ErrInvalidToken
	}
	points, ok := parseClickAnswer(answer)
	if !ok || len(points) != len(state.Points) {
		return p.fail(ctx, id, state)
	}
	for i := range state.Points {
		if absInt(points[i].X-state.Points[i].X) > state.Tolerance || absInt(points[i].Y-state.Points[i].Y) > state.Tolerance {
			return p.fail(ctx, id, state)
		}
	}
	return p.store.Delete(ctx, id)
}

func (p *ClickProvider) fail(ctx context.Context, id string, state clickState) error {
	state.Attempts++
	if state.Attempts >= state.MaxAttempts {
		_ = p.store.Delete(ctx, id)
		return ErrInvalidToken
	}
	raw, err := json.Marshal(state)
	if err == nil {
		ttl := time.Until(state.ExpiresAt)
		if ttl > 0 {
			_ = p.store.Set(ctx, id, string(raw), ttl)
		}
	}
	return ErrInvalidToken
}

func randomClickPoint(existing []point, width int, height int, tolerance int) point {
	margin := maxInt(28, tolerance+8)
	for i := 0; i < 32; i++ {
		pt := point{
			X: margin + randomInt(maxInt(1, width-margin*2)),
			Y: margin + randomInt(maxInt(1, height-margin*2)),
		}
		if farEnough(pt, existing, margin*2) {
			return pt
		}
	}
	return point{X: margin + randomInt(maxInt(1, width-margin*2)), Y: margin + randomInt(maxInt(1, height-margin*2))}
}

func farEnough(pt point, existing []point, distance int) bool {
	for _, current := range existing {
		if absInt(pt.X-current.X) < distance && absInt(pt.Y-current.Y) < distance {
			return false
		}
	}
	return true
}

func drawClickTarget(rgba *image.RGBA, pt point, label string) {
	radius := 14
	drawCircle(rgba, pt.X, pt.Y, radius, color.RGBA{R: 255, G: 255, B: 255, A: 130})
	drawCircle(rgba, pt.X, pt.Y, radius-3, color.RGBA{R: 28, G: 94, B: 180, A: 180})
	d := font.Drawer{
		Dst:  rgba,
		Src:  imageColor(color.RGBA{R: 255, G: 255, B: 255, A: 255}),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(pt.X-4, pt.Y+5),
	}
	d.DrawString(label)
}

type imageColor color.RGBA

func (c imageColor) ColorModel() color.Model { return color.RGBAModel }
func (c imageColor) Bounds() image.Rectangle { return image.Rect(0, 0, 1, 1) }
func (c imageColor) At(x int, y int) color.Color {
	return color.RGBA(c)
}
