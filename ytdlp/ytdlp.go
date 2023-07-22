package ytdlp

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

const (
	ffmpegPath = "ffmpeg"
	ytDlpPath  = "yt-dlp"
)

type FetchResult struct {
	rawInfo []byte
}

func Fetch(ctx context.Context, url string) (*FetchResult, error) {
	metadataCmd := exec.CommandContext(
		ctx,
		ytDlpPath,
		// see comment below about ignoring errors for playlists
		"--ignore-errors",
		"--no-call-home",
		"--no-cache-dir",
		"--skip-download",
		"--restrict-filenames",
		// provide URL via stdin for security, youtube-dl has some run command args
		"--batch-file", "-",
		"-J",
	)

	var infoBuf bytes.Buffer
	var errBuf bytes.Buffer

	metadataCmd.Stdin = strings.NewReader(url)
	metadataCmd.Stdout = &infoBuf
	metadataCmd.Stderr = &errBuf

	if err := metadataCmd.Run(); err != nil {
		return nil, err
	}

	return &FetchResult{
		rawInfo: infoBuf.Bytes(),
	}, nil
}

func (fr *FetchResult) Download(ctx context.Context, w io.WriteCloser) error {
	ytDlpCmd := exec.CommandContext(
		ctx,
		ytDlpPath,
		"--no-call-home",
		"--no-cache-dir",
		"--ignore-errors",
		"--newline",
		"--restrict-filenames",
		"--load-info", "-",
		"-x",
		// https://github.com/yt-dlp/yt-dlp/issues/979#issuecomment-919629354
		"-f", "ba/ba*",
		"--format-sort", "aext:opus",
		"-g",
	)
	ytDlpCmd.Cancel = func() error {
		return ytDlpCmd.Process.Signal(os.Interrupt)
	}

	var buf bytes.Buffer
	ytDlpCmd.Stdin = bytes.NewReader(fr.rawInfo)
	ytDlpCmd.Stdout = &buf
	ytDlpCmd.Stderr = io.Discard

	if err := ytDlpCmd.Run(); err != nil {
		return err
	}

	resultURL, err := url.ParseRequestURI(strings.TrimSpace(buf.String()))
	if err != nil {
		return err
	}

	if err := downloadOpus(ctx, resultURL, w); err != nil {
		return err
	}

	return nil
}

func downloadOpus(ctx context.Context, fileURL *url.URL, w io.WriteCloser) error {
	ffmpegCmd := exec.CommandContext(
		ctx,
		ffmpegPath,
		"-i", fileURL.String(),
		"-vn",
		"-acodec", "libopus",
		"-f", "ogg",
		"pipe:",
	)
	ffmpegCmd.Cancel = func() error {
		defer w.Close()
		return ffmpegCmd.Process.Signal(os.Interrupt)
	}
	ffmpegCmd.Stdout = w
	ffmpegCmd.Stderr = io.Discard

	if err := ffmpegCmd.Run(); err != nil {
		return err
	}

	return ctx.Err()
}
