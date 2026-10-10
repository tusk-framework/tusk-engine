package cli

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

var documentationVersion = "dev"

func runDocs(args []string, output io.Writer, files fs.FS, version string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: tusk docs [topic]; available topics: %s", documentationTopics(files))
	}

	topic := "index"
	if len(args) == 1 {
		topic = strings.TrimSuffix(args[0], ".md")
		if topic == "" || strings.ContainsAny(topic, `/\\`) || !fs.ValidPath(topic) || path.Base(topic) != topic {
			return fmt.Errorf("invalid documentation topic %q; available topics: %s", args[0], documentationTopics(files))
		}
	}

	content, err := fs.ReadFile(files, path.Join("user-guide", topic+".md"))
	if err != nil {
		return fmt.Errorf("unknown documentation topic %q; available topics: %s", topic, documentationTopics(files))
	}
	if strings.TrimSpace(string(content)) == "" {
		return fmt.Errorf("documentation topic %q is empty", topic)
	}
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	if _, err := fmt.Fprintf(output, "Tusk Engine documentation (%s)\n\n", version); err != nil {
		return err
	}
	if _, err := output.Write(content); err != nil {
		return err
	}
	if content[len(content)-1] != '\n' {
		_, err = io.WriteString(output, "\n")
	}
	return err
}

func documentationTopics(files fs.FS) string {
	entries, err := fs.ReadDir(files, "user-guide")
	if err != nil {
		return "unavailable"
	}
	topics := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == "index.md" || !strings.HasSuffix(name, ".md") {
			continue
		}
		topics = append(topics, strings.TrimSuffix(name, ".md"))
	}
	if len(topics) == 0 {
		return "none"
	}
	return strings.Join(topics, ", ")
}
