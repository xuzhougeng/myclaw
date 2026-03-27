package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"myclaw/internal/app"
)

const scannerMaxTokenSize = 1024 * 1024

type REPL struct {
	service *app.Service
	input   io.Reader
	output  io.Writer
}

func NewREPL(service *app.Service, input io.Reader, output io.Writer) *REPL {
	return &REPL{
		service: service,
		input:   input,
		output:  output,
	}
}

func (r *REPL) Run(ctx context.Context) error {
	fmt.Fprintf(r.output, "myclaw terminal\n")
	fmt.Fprintln(r.output, "model config is read only from local env vars")
	fmt.Fprintln(r.output, "required vars: MYCLAW_MODEL_PROVIDER, MYCLAW_MODEL_BASE_URL, MYCLAW_MODEL_API_KEY, MYCLAW_MODEL_NAME")
	fmt.Fprintln(r.output, "type /help for commands")

	scanner := bufio.NewScanner(r.input)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerMaxTokenSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Fprint(r.output, "myclaw> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		case line == "/exit" || line == "/quit":
			return nil
		case line == "/help":
			r.printHelp()
		case line == "/remember":
			body, err := r.readMultiline(scanner, "记忆内容")
			if err != nil {
				return err
			}
			r.runMessage(ctx, "/remember "+body)
		case line == "/ask":
			body, err := r.readMultiline(scanner, "问题")
			if err != nil {
				return err
			}
			r.runMessage(ctx, body)
		default:
			r.runMessage(ctx, line)
		}
	}
}

func (r *REPL) runMessage(ctx context.Context, message string) {
	reply, err := r.service.HandleMessage(ctx, app.MessageContext{
		UserID:    "terminal",
		Interface: "terminal",
	}, message)
	if err != nil {
		fmt.Fprintf(r.output, "error: %v\n", err)
		return
	}
	fmt.Fprintf(r.output, "%s\n", strings.TrimSpace(reply))
}

func (r *REPL) readMultiline(scanner *bufio.Scanner, title string) (string, error) {
	fmt.Fprintf(r.output, "paste %s, end with EOF on its own line\n", title)
	var lines []string
	for {
		fmt.Fprint(r.output, "... ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			break
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "EOF" {
			break
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func (r *REPL) printHelp() {
	fmt.Fprintln(r.output, "commands:")
	fmt.Fprintln(r.output, "  /help")
	fmt.Fprintln(r.output, "  /exit")
	fmt.Fprintln(r.output, "  /remember          paste multiline content until EOF")
	fmt.Fprintln(r.output, "  /append <ID> <内容>")
	fmt.Fprintln(r.output, "  /ask               paste multiline question until EOF")
	fmt.Fprintln(r.output, "  /notice 2小时后 喝水")
	fmt.Fprintln(r.output, "  /notice 每天 09:00 写日报")
	fmt.Fprintln(r.output, "  /notice 2026-03-30 14:00 交房租")
	fmt.Fprintln(r.output, "  /notice list")
	fmt.Fprintln(r.output, "  /notice remove <提醒ID前缀>")
	fmt.Fprintln(r.output, "  /cron ...           same as /notice")
	fmt.Fprintln(r.output, "  /list /stats /clear")
}
