package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"baize/internal/app"
)

const scannerMaxTokenSize = 1024 * 1024

type REPL struct {
	service *app.Service
	input   io.Reader
	output  io.Writer
	session string
}

func NewREPL(service *app.Service, input io.Reader, output io.Writer) *REPL {
	return &REPL{
		service: service,
		input:   input,
		output:  output,
		session: "terminal-repl",
	}
}

func (r *REPL) Run(ctx context.Context) error {
	fmt.Fprintf(r.output, "baize terminal\n")
	fmt.Fprintln(r.output, "model config is loaded from the local model database under the data directory")
	fmt.Fprintln(r.output, "type /help for commands")

	scanner := bufio.NewScanner(r.input)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerMaxTokenSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Fprint(r.output, "baize> ")
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
		case app.IsNewConversationCommand(line):
			r.session = fmt.Sprintf("terminal-repl:%d", time.Now().UnixNano())
			fmt.Fprintln(r.output, "已开启新对话。")
		case line == "/help":
			r.printHelp()
		case line == "/kb remember":
			body, err := r.readMultiline(scanner, "记忆内容")
			if err != nil {
				return err
			}
			r.runMessage(ctx, "/kb remember "+body)
		case line == "/translate":
			body, err := r.readMultiline(scanner, "待翻译内容")
			if err != nil {
				return err
			}
			r.runMessage(ctx, "/translate "+body)
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
		SessionID: r.session,
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
	fmt.Fprintln(r.output, "  /new")
	fmt.Fprintln(r.output, "  /exit")
	fmt.Fprintln(r.output, "  /kb                show current knowledge base and available ones")
	fmt.Fprintln(r.output, "  /kb new <名称>")
	fmt.Fprintln(r.output, "  /kb switch <名称>")
	fmt.Fprintln(r.output, "  /kb remember       paste multiline content until EOF")
	fmt.Fprintln(r.output, "  /kb remember-file <路径>")
	fmt.Fprintln(r.output, "  /kb append <ID> <内容>")
	fmt.Fprintln(r.output, "  /kb forget <ID>")
	fmt.Fprintln(r.output, "  /kb list /kb stats /kb clear")
	fmt.Fprintln(r.output, "  /skill")
	fmt.Fprintln(r.output, "  /skill list")
	fmt.Fprintln(r.output, "  /skill show <技能名>")
	fmt.Fprintln(r.output, "  /skill load <技能名>")
	fmt.Fprintln(r.output, "  /skill unload <技能名>")
	fmt.Fprintln(r.output, "  /skill clear")
	fmt.Fprintln(r.output, "  /translate         paste multiline content until EOF")
	fmt.Fprintln(r.output, "  /ask               paste multiline question until EOF")
	fmt.Fprintln(r.output, "  /notice 2小时后 喝水")
	fmt.Fprintln(r.output, "  /notice 每天 09:00 写日报")
	fmt.Fprintln(r.output, "  /notice 2026-03-30 14:00 交房租")
	fmt.Fprintln(r.output, "  /notice list")
	fmt.Fprintln(r.output, "  /notice remove <提醒ID前缀>")
	fmt.Fprintln(r.output, "  /cron ...           same as /notice")
}
