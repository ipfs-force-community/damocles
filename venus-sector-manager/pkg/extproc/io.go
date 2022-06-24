package extproc

import (
	"bufio"
	"encoding/json"
	"fmt"
)

func ReadyMessage(taskName string) string {
	return fmt.Sprintf("%s processor ready\n", taskName)
}

func WriteReadyMessage(w *bufio.Writer, taskName string) error {
	_, err := w.WriteString(ReadyMessage(taskName))
	if err != nil {
		return fmt.Errorf("write ready message: %w", err)
	}

	err = w.Flush()
	if err != nil {
		return fmt.Errorf("flush ready message: %w", err)
	}

	return nil
}

func WriteData(w *bufio.Writer, data interface{}) (int, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("marshal data object: %w", err)
	}

	n, err := w.Write(b)
	if err != nil {
		return 0, fmt.Errorf("write bytes: %w", err)
	}

	err = w.WriteByte('\n')
	if err != nil {
		return 0, fmt.Errorf("write newline: %w", err)
	}

	err = w.Flush()
	if err != nil {
		return 0, fmt.Errorf("flush: %w", err)
	}

	if n != len(b) {
		log.Warnf("unexpected written bytes, expected %d, written %d", len(b), n)
	}

	return n, nil
}
