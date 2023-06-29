package ytdlp

import (
	"bytes"
	"context"
	"io"
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

	metadataCmd.Stdin = strings.NewReader(url)
	metadataCmd.Stdout = &infoBuf
	metadataCmd.Stderr = os.Stderr

	if err := metadataCmd.Run(); err != nil {
		return nil, err
	}

	return &FetchResult{
		rawInfo: infoBuf.Bytes(),
	}, nil
}

func (fr *FetchResult) Download(ctx context.Context, w io.Writer) error {
	ffmpegStdin, ytDlpStdout := io.Pipe()

	ffmpegCmd := exec.CommandContext(
		ctx,
		ffmpegPath,
		"-i", "pipe:",
		"-vn",
		"-acodec", "libopus",
		"-f", "webm",
		"pipe:",
	)
	ffmpegCmd.Stdin = ffmpegStdin
	ffmpegCmd.Stdout = w
	ffmpegCmd.Stderr = os.Stderr

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
		"-o", "-",
	)
	ytDlpCmd.Stdin = bytes.NewReader(fr.rawInfo)
	ytDlpCmd.Stdout = ytDlpStdout
	ytDlpCmd.Stderr = os.Stderr

	if err := ffmpegCmd.Start(); err != nil {
		return err
	}
	if err := ytDlpCmd.Start(); err != nil {
		return err
	}

	_ = ytDlpCmd.Wait()
	ytDlpStdout.Close()

	_ = ffmpegCmd.Wait()

	return nil
}
