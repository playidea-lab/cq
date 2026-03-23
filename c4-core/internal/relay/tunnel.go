package relay

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// HandleTunnel connects to the relay server as a tunnel receiver and pipes
// incoming binary WebSocket frames into tar's stdin for extraction.
//
// relayURL is the WebSocket URL of the relay server (ws:// or wss://).
// tunnelID identifies the tunnel session on the relay server.
// destPath is the local directory where tar will extract files.
// token is the auth token for the relay server.
//
// The function dials {relayURL}/tunnel/{tunnelID}?role=receiver, then
// starts "tar xf - -C destPath" and pipes each binary frame to its stdin.
// It returns when the WebSocket closes or an error occurs.
func HandleTunnel(relayURL, tunnelID, destPath, token string) error {
	// Ensure destPath exists.
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("tunnel: create dest dir %q: %w", destPath, err)
	}

	// Build tunnel WebSocket URL.
	u, err := url.Parse(relayURL)
	if err != nil {
		return fmt.Errorf("tunnel: invalid relay URL %q: %w", relayURL, err)
	}

	// Normalise scheme: wss → wss, ws → ws (gobwas/ws handles TLS via scheme).
	scheme := strings.ToLower(u.Scheme)
	if scheme != "ws" && scheme != "wss" {
		return fmt.Errorf("tunnel: unsupported URL scheme %q (want ws or wss)", u.Scheme)
	}

	u.Path = "/tunnel/" + tunnelID
	q := u.Query()
	q.Set("role", "receiver")
	if token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()

	// Dial the relay server.
	// gobwas/ws may read ahead during the HTTP upgrade handshake and return
	// buffered bytes in the second return value. We must use that buffer as
	// the read source so that frames sent immediately after the handshake
	// are not lost.
	dialer := ws.Dialer{Timeout: 15 * time.Second}
	ctx := context.Background()
	conn, buf, _, err := dialer.Dial(ctx, u.String())
	if err != nil {
		return fmt.Errorf("tunnel: dial %s: %w", u.String(), err)
	}
	defer conn.Close()

	// Build an io.ReadWriter that reads through the buffered reader (which
	// may contain bytes already consumed from conn during the handshake),
	// and writes directly to conn.
	rw := newConnRW(conn, buf)

	// Start tar process.
	tarCmd := exec.Command("tar", "xf", "-", "-C", destPath)
	tarStdin, err := tarCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("tunnel: tar stdin pipe: %w", err)
	}
	tarCmd.Stdout = os.Stdout
	tarCmd.Stderr = os.Stderr

	if err := tarCmd.Start(); err != nil {
		return fmt.Errorf("tunnel: start tar: %w", err)
	}

	// Pipe WebSocket binary frames to tar stdin.
	pipeErr := func() error {
		defer tarStdin.Close()
		for {
			data, op, err := wsutil.ReadServerData(rw)
			if err != nil {
				if isWSCloseError(err) || err == io.EOF {
					return nil
				}
				return fmt.Errorf("tunnel: ws read: %w", err)
			}
			if op == ws.OpClose {
				return nil
			}
			if op != ws.OpBinary {
				continue
			}
			if _, err := tarStdin.Write(data); err != nil {
				return fmt.Errorf("tunnel: write to tar: %w", err)
			}
		}
	}()

	// Wait for tar to finish.
	waitErr := tarCmd.Wait()

	if pipeErr != nil {
		return pipeErr
	}
	if waitErr != nil {
		return fmt.Errorf("tunnel: tar exited: %w", waitErr)
	}
	return nil
}

// connRW is an io.ReadWriter that reads via a (possibly buffered) reader
// and writes directly to the underlying net.Conn.
type connRW struct {
	r io.Reader
	w io.Writer
}

// newConnRW builds a connRW. If buf is non-nil and has unread bytes, reads
// drain buf first before reading from conn. Otherwise reads go straight to conn.
func newConnRW(conn net.Conn, buf *bufio.Reader) *connRW {
	var r io.Reader = conn
	if buf != nil && buf.Buffered() > 0 {
		// Drain buf then fall through to conn.
		r = io.MultiReader(buf, conn)
	}
	return &connRW{r: r, w: conn}
}

func (c *connRW) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *connRW) Write(p []byte) (int, error) { return c.w.Write(p) }

// isWSCloseError reports whether err signals a normal WebSocket close or
// end-of-stream (connection closed by peer without a formal close frame).
func isWSCloseError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "ws: close") ||
		strings.Contains(s, "ws closed") ||
		strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "use of reserved op code") ||
		strings.Contains(s, "EOF")
}
