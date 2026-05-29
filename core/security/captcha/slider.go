package captcha

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"time"

	"github.com/huwenlong92/sdkit/core/security/captcha/store"
	"github.com/huwenlong92/sdkit/pkg/security/token"
)

const (
	defaultSliderWidth       = 320
	defaultSliderHeight      = 160
	defaultSliderPieceSize   = 42
	defaultSliderTolerance   = 6
	defaultSliderMaxAttempts = 3
	defaultSliderMinDuration = 250 * time.Millisecond
)

type SliderOption func(*sliderOptions)

type sliderOptions struct {
	width       int
	height      int
	pieceSize   int
	tolerance   int
	maxAttempts int
	minDuration time.Duration
	source      BackgroundSource
}

type SliderProvider struct {
	store store.Store
	ttl   time.Duration
	opts  sliderOptions
}

type sliderState struct {
	X           int       `json:"x"`
	Y           int       `json:"y"`
	Tolerance   int       `json:"tolerance"`
	MaxAttempts int       `json:"max_attempts"`
	Attempts    int       `json:"attempts"`
	MinDuration int64     `json:"min_duration_ms"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func NewSliderProvider(store store.Store, ttl time.Duration, opts ...SliderOption) *SliderProvider {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	options := sliderOptions{
		width:       defaultSliderWidth,
		height:      defaultSliderHeight,
		pieceSize:   defaultSliderPieceSize,
		tolerance:   defaultSliderTolerance,
		maxAttempts: defaultSliderMaxAttempts,
		minDuration: defaultSliderMinDuration,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.width <= 0 {
		options.width = defaultSliderWidth
	}
	if options.height <= 0 {
		options.height = defaultSliderHeight
	}
	if options.pieceSize <= 0 {
		options.pieceSize = defaultSliderPieceSize
	}
	if options.tolerance < 0 {
		options.tolerance = defaultSliderTolerance
	}
	if options.maxAttempts <= 0 {
		options.maxAttempts = defaultSliderMaxAttempts
	}
	return &SliderProvider{store: store, ttl: ttl, opts: options}
}

func WithSliderSize(width int, height int) SliderOption {
	return func(o *sliderOptions) {
		o.width = width
		o.height = height
	}
}

func WithSliderPieceSize(size int) SliderOption {
	return func(o *sliderOptions) {
		o.pieceSize = size
	}
}

func WithSliderTolerance(tolerance int) SliderOption {
	return func(o *sliderOptions) {
		o.tolerance = tolerance
	}
}

func WithSliderMaxAttempts(maxAttempts int) SliderOption {
	return func(o *sliderOptions) {
		o.maxAttempts = maxAttempts
	}
}

func WithSliderMinDuration(duration time.Duration) SliderOption {
	return func(o *sliderOptions) {
		o.minDuration = duration
	}
}

func WithSliderBackgroundSource(source BackgroundSource) SliderOption {
	return func(o *sliderOptions) {
		o.source = source
	}
}

func (p *SliderProvider) Name() string { return "slider" }

func (p *SliderProvider) Kind() Kind { return KindSlider }

func (p *SliderProvider) Generate(ctx context.Context, opts GenerateOptions) (*Challenge, error) {
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
	pieceSize := minInt(p.opts.pieceSize, minInt(p.opts.width/3, p.opts.height/2))
	x := pieceSize + randomInt(maxInt(1, p.opts.width-pieceSize*2))
	y := randomInt(maxInt(1, p.opts.height-pieceSize))

	piece := image.NewRGBA(image.Rect(0, 0, pieceSize, pieceSize))
	for py := 0; py < pieceSize; py++ {
		for px := 0; px < pieceSize; px++ {
			piece.SetRGBA(px, py, bg.RGBAAt(x+px, y+py))
		}
	}
	drawRect(piece, image.Rect(0, 0, pieceSize, 2), color.RGBA{R: 255, G: 255, B: 255, A: 160})
	drawRect(piece, image.Rect(0, pieceSize-2, pieceSize, pieceSize), color.RGBA{R: 0, G: 0, B: 0, A: 80})
	drawRect(piece, image.Rect(0, 0, 2, pieceSize), color.RGBA{R: 255, G: 255, B: 255, A: 120})
	drawRect(piece, image.Rect(pieceSize-2, 0, pieceSize, pieceSize), color.RGBA{R: 0, G: 0, B: 0, A: 80})

	gapped := cloneRGBA(bg)
	drawRect(gapped, image.Rect(x, y, x+pieceSize, y+pieceSize), color.RGBA{R: 245, G: 245, B: 245, A: 160})
	drawRect(gapped, image.Rect(x, y, x+pieceSize, y+2), color.RGBA{R: 255, G: 255, B: 255, A: 200})
	drawRect(gapped, image.Rect(x, y+pieceSize-2, x+pieceSize, y+pieceSize), color.RGBA{R: 0, G: 0, B: 0, A: 80})

	imageData, err := encodePNGDataURL(gapped)
	if err != nil {
		return nil, err
	}
	pieceData, err := encodePNGDataURL(piece)
	if err != nil {
		return nil, err
	}
	id, err := token.RandomToken(16)
	if err != nil {
		return nil, err
	}
	state := sliderState{
		X:           x,
		Y:           y,
		Tolerance:   p.opts.tolerance,
		MaxAttempts: p.opts.maxAttempts,
		MinDuration: p.opts.minDuration.Milliseconds(),
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
		Kind:     KindSlider,
		Image:    imageData,
		ExpireAt: state.ExpiresAt,
		Payload: map[string]any{
			"piece":           pieceData,
			"width":           p.opts.width,
			"height":          p.opts.height,
			"piece_width":     pieceSize,
			"piece_height":    pieceSize,
			"piece_y":         y,
			"tolerance":       p.opts.tolerance,
			"min_duration_ms": p.opts.minDuration.Milliseconds(),
		},
	}, nil
}

func (p *SliderProvider) Verify(ctx context.Context, id string, answer string) error {
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
	var state sliderState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		_ = p.store.Delete(ctx, id)
		return ErrInvalidToken
	}
	if time.Now().After(state.ExpiresAt) {
		_ = p.store.Delete(ctx, id)
		return ErrInvalidToken
	}
	point, duration, ok := parseSliderAnswer(answer)
	if !ok || absInt(point.X-state.X) > state.Tolerance {
		return p.fail(ctx, id, state)
	}
	if point.Y > 0 && absInt(point.Y-state.Y) > state.Tolerance {
		return p.fail(ctx, id, state)
	}
	if state.MinDuration > 0 && int64(duration) < state.MinDuration {
		return p.fail(ctx, id, state)
	}
	return p.store.Delete(ctx, id)
}

func (p *SliderProvider) fail(ctx context.Context, id string, state sliderState) error {
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
