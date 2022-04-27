package ext

import (
	"bufio"
	"encoding/json"
	"fmt"
)

func WriteData(w *bufio.Writer, data interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal data object: %w", err)
	}

	_, err = w.Write(b)
	if err != nil {
		return fmt.Errorf("write bytes: %w", err)
	}

	err = w.WriteByte('\n')
	if err != nil {
		return fmt.Errorf("write newline: %w", err)
	}

	err = w.Flush()
	if err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	return nil
}
