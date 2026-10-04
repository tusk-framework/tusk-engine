package cli

import (
	"fmt"
	"io"

	"github.com/tusk-framework/tusk-engine/internal/migration"
)

func writeProjectDiagnosis(output io.Writer, project migration.Detection) error {
	if _, err := fmt.Fprintf(output, "\nProject: %s\n", project.Message); err != nil {
		return err
	}
	if project.Action != "" {
		_, err := fmt.Fprintf(output, "Action: %s\n", project.Action)
		return err
	}
	return nil
}
