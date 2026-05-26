package imapstats

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

type Shipper struct {
	reg        *metrics.Registry
	unit       string
	logger     *log.Logger
	cmdBuilder func(unit string) *exec.Cmd
}

func NewShipper(reg *metrics.Registry, unit string, logger *log.Logger) *Shipper {
	return &Shipper{
		reg:    reg,
		unit:   unit,
		logger: logger,
		cmdBuilder: func(unit string) *exec.Cmd {
			return exec.Command("journalctl", "-u", unit, "-f", "-o", "json", "--since=now")
		},
	}
}

func (s *Shipper) Run(ctx context.Context) error {
	if s == nil || strings.TrimSpace(s.unit) == "" {
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		cmd := s.cmdBuilder(s.unit)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		cmd.Stderr = io.Discard

		if err := cmd.Start(); err != nil {
			return err
		}
		processDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			case <-processDone:
			}
		}()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var entry struct {
				Message string `json:"MESSAGE"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				if s.logger != nil {
					s.logger.Printf("imapstats journal json: %v", err)
				}
				continue
			}
			if isImapLogin(entry.Message) {
				s.reg.Counter("imap_login").Add(1)
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil && s.logger != nil {
			s.logger.Printf("imapstats journal scan: %v", err)
		}

		waitErr := cmd.Wait()
		close(processDone)
		if err := ctx.Err(); err != nil {
			return err
		}
		if waitErr != nil && s.logger != nil {
			s.logger.Printf("imapstats journal exited: %v", waitErr)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func isImapLogin(message string) bool {
	return strings.Contains(message, "imap-login:") && strings.Contains(message, " Login: user=")
}
