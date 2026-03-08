package agent

import (
	"fmt"
	"iter"
)

func consumeRunEvents[T any](seq iter.Seq2[T, error], runFailureMessage string, handle func(T)) error {
	for event, err := range seq {
		if err != nil {
			return fmt.Errorf("%s: %w", runFailureMessage, err)
		}
		if handle != nil {
			handle(event)
		}
	}

	return nil
}
