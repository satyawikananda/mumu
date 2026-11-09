// Command hlsfixture serves an original synthetic live HLS test pattern on loopback.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func main() {
	resolution := flag.Int("resolution", 720, "720 or 1080")
	port := flag.Int("port", 18765, "Loopback HTTP port")
	duration := flag.Int("seconds", 180, "Fixture duration, 10..600 seconds")
	continuous := flag.Bool("continuous-audio", false, "Add a quiet continuous 220 Hz tone to measure audio gaps")
	flag.Parse()
	if err := run(*resolution, *port, *duration, *continuous); err != nil {
		log.Fatal(err)
	}
}

func run(resolution, port, seconds int, continuous bool) error {
	if resolution != 720 && resolution != 1080 {
		return fmt.Errorf("resolution must be 720 or 1080")
	}
	if port < 1024 || port > 65535 {
		return fmt.Errorf("port must be 1024..65535")
	}
	if seconds < 10 || seconds > 600 {
		return fmt.Errorf("seconds must be 10..600")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds+10)*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "mumu-hls-*")
	if err != nil {
		return fmt.Errorf("fixture directory: %w", err)
	}
	defer os.RemoveAll(dir)
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	server := &http.Server{Handler: http.FileServer(http.Dir(dir)), ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(listener) }()
	defer server.Close()
	width := resolution * 16 / 9
	audio := `if(lt(mod(t\,1)\,0.05)\,0.2*sin(2*PI*880*t)\,0)`
	if continuous {
		audio += "+0.02*sin(2*PI*220*t)"
	}
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error", "-re", "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=30", width, resolution),
		"-f", "lavfi", "-i", "aevalsrc="+audio+":s=48000",
		"-vf", `drawbox=x=0:y=0:w=160:h=160:color=white:t=fill:enable=lt(mod(t\,1)\,0.05)`,
		"-t", strconv.Itoa(seconds), "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "60",
		"-c:a", "aac", "-b:a", "128k", "-f", "hls", "-hls_time", "2", "-hls_list_size", "6",
		"-hls_flags", "delete_segments+omit_endlist+independent_segments", filepath.Join(dir, "index.m3u8"),
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	processErr := make(chan error, 1)
	go func() { processErr <- cmd.Wait() }()
	log.Printf("original synthetic live HLS: http://127.0.0.1:%d/index.m3u8 (%dp); wait for READY", port, resolution)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	for {
		select {
		case <-ctx.Done():
			<-processErr
			return nil
		case err := <-processErr:
			if err != nil {
				return fmt.Errorf("ffmpeg: %w", err)
			}
			return nil
		case err := <-serverErr:
			cancel()
			<-processErr
			if !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("serve: %w", err)
			}
			return nil
		case <-ticker.C:
			if !ready {
				if _, err := os.Stat(filepath.Join(dir, "index.m3u8")); err == nil {
					log.Print("READY")
					ready = true
				}
			}
		}
	}
}
