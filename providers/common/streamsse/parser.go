package streamsse

import (
	"bufio"
	"io"
	"strings"
)

// Event captures one SSE event envelope.
type Event struct {
	Event string
	Data  string
}

// Parse reads server-sent events from reader and invokes fn for each complete event.
func Parse(reader io.Reader, fn func(Event) error) error {
	scanner := bufio.NewScanner(reader)
	// Allow moderately large payload lines.
	scanner.Buffer(make([]byte, 0, 4096), 512*1024)

	var eventName string
	var dataLines []string

	flush := func() error {
		if len(dataLines) == 0 {
			eventName = ""
			return nil
		}
		ev := Event{
			Event: strings.TrimSpace(eventName),
			Data:  strings.Join(dataLines, "\n"),
		}
		eventName = ""
		dataLines = dataLines[:0]
		return fn(ev)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}
