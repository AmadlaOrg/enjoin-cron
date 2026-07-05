package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AmadlaOrg/enjoin-cron/cron"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	appName = "enjoin-cron"
	version = "1.0.0"
)

var rootCmd = &cobra.Command{
	Use:     appName,
	Short:   "Enjoin plugin for cron job and scheduled task management",
	Version: version,
}

var (
	infoOutputFlag string
	infoHeryFlag   bool

	infoCmd = &cobra.Command{
		Use:   "info",
		Short: "Show plugin metadata",
		Run: func(cmd *cobra.Command, args []string) {
			metadata := map[string]any{
				"name":        appName,
				"version":     version,
				"engine":      "cron",
				"supports":    []string{"amadla.org/entity/cron@^v1.0.0"},
				"description": "Manages cron jobs and systemd timers",
			}
			if err := writeInfoOutput(os.Stdout, infoOutputFlag, infoHeryFlag, metadata); err != nil {
				fmt.Fprintf(os.Stderr, "Error encoding metadata: %v\n", err)
				os.Exit(1)
			}
		},
	}
)

var (
	applyFilePath    string
	validateFilePath string

	applyCmd = &cobra.Command{
		Use:   "apply",
		Short: "Apply cron job configuration",
		RunE:  runApply,
	}

	validateCmd = &cobra.Command{
		Use:   "validate",
		Short: "Validate cron job configuration without changes",
		RunE:  runValidate,
	}
)

func init() {
	applyCmd.Flags().StringVarP(&applyFilePath, "file", "f", "", "Input data file (JSON or YAML; use '-' for stdin)")
	validateCmd.Flags().StringVarP(&validateFilePath, "file", "f", "", "Input data file (JSON or YAML; use '-' for stdin)")
	infoCmd.Flags().StringVarP(&infoOutputFlag, "output", "o", "table", "Output format: table, json, yaml")
	infoCmd.Flags().BoolVar(&infoHeryFlag, "hery", false, "Wrap output in HERY envelope (_type, _body)")
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(validateCmd)
}

// readInput reads from file or stdin.
func readInput(filePath string) ([]byte, error) {
	if filePath == "" || filePath == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(filePath)
}

// parseInput auto-detects JSON or YAML.
func parseInput(data []byte) (*cron.Config, error) {
	trimmed := strings.TrimSpace(string(data))
	var cfg cron.Config
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}
	}
	return &cfg, nil
}

func runApply(cmd *cobra.Command, args []string) error {
	input, err := readInput(applyFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
		os.Exit(2)
		return nil
	}

	if len(strings.TrimSpace(string(input))) == 0 {
		fmt.Fprintln(os.Stderr, "error: empty input")
		os.Exit(2)
		return nil
	}

	cfg, err := parseInput(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing input: %v\n", err)
		os.Exit(2)
		return nil
	}

	result := cron.Apply(cfg)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding output: %v\n", err)
		os.Exit(2)
		return nil
	}
	if !result.Success {
		os.Exit(1)
	}
	return nil
}

func runValidate(cmd *cobra.Command, args []string) error {
	input, err := readInput(validateFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
		os.Exit(2)
		return nil
	}

	if len(strings.TrimSpace(string(input))) == 0 {
		fmt.Fprintln(os.Stderr, "error: empty input")
		os.Exit(2)
		return nil
	}

	cfg, err := parseInput(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing input: %v\n", err)
		os.Exit(2)
		return nil
	}

	result := cron.Validate(cfg)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding output: %v\n", err)
		os.Exit(2)
		return nil
	}
	if !result.Success {
		os.Exit(1)
	}
	return nil
}

type heryEnvelope struct {
	Type string `json:"_type" yaml:"_type"`
	Body any    `json:"_body" yaml:"_body"`
}

func writeInfoOutput(w io.Writer, format string, hery bool, data map[string]any) error {
	if hery {
		return writeHeryOutput(w, format, data)
	}

	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "yaml":
		bytes, err := yaml.Marshal(data)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(w, string(bytes))
		return err
	default:
		table := tablewriter.NewWriter(w)
		table.Header("Field", "Value")
		table.Append("Name", fmt.Sprint(data["name"]))
		table.Append("Version", fmt.Sprint(data["version"]))
		table.Append("Engine", fmt.Sprint(data["engine"]))
		table.Append("Description", fmt.Sprint(data["description"]))
		if supports, ok := data["supports"].([]string); ok {
			table.Append("Supports", strings.Join(supports, "\n"))
		}
		table.Render()
		return nil
	}
}

func writeHeryOutput(w io.Writer, format string, data map[string]any) error {
	envelope := heryEnvelope{
		Type: "amadla.org/entity/tools/info@v1.0.0",
		Body: data,
	}

	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(envelope)
	case "table":
		fmt.Fprintf(w, "_type: %s\n\n", envelope.Type)
		table := tablewriter.NewWriter(w)
		table.Header("Field", "Value")
		table.Append("Name", fmt.Sprint(data["name"]))
		table.Append("Version", fmt.Sprint(data["version"]))
		table.Append("Engine", fmt.Sprint(data["engine"]))
		table.Append("Description", fmt.Sprint(data["description"]))
		if supports, ok := data["supports"].([]string); ok {
			table.Append("Supports", strings.Join(supports, "\n"))
		}
		table.Render()
		return nil
	default:
		bytes, err := yaml.Marshal(envelope)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(w, string(bytes))
		return err
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
