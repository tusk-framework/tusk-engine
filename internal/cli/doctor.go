package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type projectDiagnosis struct {
	State   string `json:"state"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

func detectProject(root string) (projectDiagnosis, error) {
	bootstrap := filepath.Join(root, "bootstrap", "app.php")
	info, err := os.Stat(bootstrap)
	if err == nil {
		if !info.Mode().IsRegular() {
			return projectDiagnosis{}, fmt.Errorf("bootstrap path %s is not a regular file", bootstrap)
		}
		return projectDiagnosis{
			State:   "modern",
			Message: "Modern bootstrap/app.php is present.",
			Action:  "Run tusk start after confirming the application bootstrap and dependencies.",
		}, nil
	}
	if !os.IsNotExist(err) {
		return projectDiagnosis{}, fmt.Errorf("inspect %s: %w", bootstrap, err)
	}
	return projectDiagnosis{
		State:   "missing",
		Message: "No bootstrap/app.php was found; the Engine requires the modern application contract.",
		Action:  "Add bootstrap/app.php from the modern application skeleton, then run tusk doctor or tusk start.",
	}, nil
}

func writeProjectDiagnosis(output io.Writer, project projectDiagnosis) error {
	if _, err := fmt.Fprintf(output, "\nProject: %s\n", project.Message); err != nil {
		return err
	}
	if project.Action != "" {
		_, err := fmt.Fprintf(output, "Action: %s\n", project.Action)
		return err
	}
	return nil
}
