package transcription

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"sync"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

const (
	DefaultModelPath = "models/ggml-base.bin"
	DefaultLanguage  = "nl"
	DefaultThreads   = 4
)

type Input struct {
	Filename    string
	ContentType string
	Data        []byte
}

type Result struct {
	Text       string
	Language   string
	Confidence *float64
}

type Client struct {
	model    whisper.Model
	language string
	threads  int
	mu       sync.Mutex
}

func NewClient() (*Client, error) {
	model, err := whisper.New(DefaultModelPath)
	if err != nil {
		return nil, fmt.Errorf("load whisper model %q: %w", DefaultModelPath, err)
	}
	return &Client{
		model:    model,
		language: DefaultLanguage,
		threads:  DefaultThreads,
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.model == nil {
		return nil
	}
	return c.model.Close()
}

func (c *Client) Name() string {
	return "whisper.cpp"
}

func (c *Client) Transcribe(ctx context.Context, input Input) (Result, error) {
	if c == nil {
		return Result{}, fmt.Errorf("audio transcription client is nil")
	}
	if len(input.Data) == 0 {
		return Result{}, fmt.Errorf("audio transcription data is empty")
	}

	samples, err := decodeAudioToF32PCM(ctx, input.Data)
	if err != nil {
		return Result{}, fmt.Errorf("audio decode: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	wctx, err := c.model.NewContext()
	if err != nil {
		return Result{}, fmt.Errorf("whisper context: %w", err)
	}

	wctx.SetThreads(uint(c.threads))
	if c.language != "" {
		if err := wctx.SetLanguage(c.language); err != nil {
			return Result{}, fmt.Errorf("set language %q: %w", c.language, err)
		}
	}

	if err := wctx.Process(samples, nil, nil, nil); err != nil {
		return Result{}, fmt.Errorf("whisper process: %w", err)
	}

	var text strings.Builder
	for {
		segment, err := wctx.NextSegment()
		if err != nil {
			break
		}
		text.WriteString(segment.Text)
	}

	result := strings.TrimSpace(text.String())
	if result == "" {
		return Result{}, fmt.Errorf("audio transcription response did not contain text")
	}

	lang := wctx.DetectedLanguage()
	return Result{
		Text:     result,
		Language: lang,
	}, nil
}

// decodeAudioToF32PCM uses ffmpeg to convert arbitrary audio (OGG/Opus, etc.)
// into raw 16 kHz mono float32 PCM samples expected by whisper.cpp.
func decodeAudioToF32PCM(ctx context.Context, data []byte) ([]float32, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", "pipe:0",
		"-f", "f32le",
		"-ar", "16000",
		"-ac", "1",
		"-loglevel", "error",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(data)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return bytesToFloat32(stdout.Bytes())
}

func bytesToFloat32(raw []byte) ([]float32, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("ffmpeg produced no audio output")
	}
	if len(raw)%4 != 0 {
		return nil, fmt.Errorf("unexpected audio data length: %d bytes", len(raw))
	}
	r := bytes.NewReader(raw)
	samples := make([]float32, len(raw)/4)
	for i := range samples {
		var bits uint32
		if err := binary.Read(r, binary.LittleEndian, &bits); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		samples[i] = math.Float32frombits(bits)
	}
	return samples, nil
}
