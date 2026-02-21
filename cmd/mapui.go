package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	mapUIStateFile  string
	mapUIScreenShot string
	mapUIAXDumpSrc  string
	mapUIAXDumpBin  string
)

var mapUICmd = &cobra.Command{
	Use:   "map-ui",
	Short: "Capture screenshot and map AX tree to JSON state",
	RunE: func(cmd *cobra.Command, args []string) error {
		statePath, err := expandPath(mapUIStateFile)
		if err != nil {
			return err
		}

		screenshotPath, err := expandPath(mapUIScreenShot)
		if err != nil {
			return err
		}

		axdumpBinPath, err := expandPath(mapUIAXDumpBin)
		if err != nil {
			return err
		}

		if err := captureScreenshot(screenshotPath); err != nil {
			return err
		}

		if err := buildAXDumpBinary(mapUIAXDumpSrc, axdumpBinPath); err != nil {
			return err
		}

		dump, err := runAXDump(axdumpBinPath)
		if err != nil {
			return err
		}

		elements := flattenAXTree(dump.Root)
		generatedAt := dump.GeneratedAt
		if generatedAt == "" {
			generatedAt = time.Now().UTC().Format(time.RFC3339)
		}

		state := &ViewState{
			Status:      "success",
			Action:      "map-ui",
			AppName:     dump.AppName,
			PID:         dump.PID,
			GeneratedAt: generatedAt,
			Screenshot:  screenshotPath,
			StateFile:   statePath,
			Elements:    elements,
		}

		if err := saveState(statePath, state); err != nil {
			return err
		}

		return outputJSON(state)
	},
}

func init() {
	mapUICmd.Flags().StringVar(&mapUIStateFile, "state-file", defaultStatePath(), "Path to store mapped UI state JSON")
	mapUICmd.Flags().StringVar(&mapUIScreenShot, "screenshot", defaultScreenshotPath(), "Path to store screenshot")
	mapUICmd.Flags().StringVar(&mapUIAXDumpSrc, "axdump-source", "tools/axdump/axdump.swift", "Path to axdump.swift source")
	mapUICmd.Flags().StringVar(&mapUIAXDumpBin, "axdump-bin", defaultAXDumpBinaryPath(), "Path to compiled axdump binary")
}

func captureScreenshot(outputPath string) error {
	if err := ensureParentDir(outputPath); err != nil {
		return fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	out, err := exec.Command("screencapture", "-x", outputPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to capture screenshot: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func buildAXDumpBinary(sourcePath, binaryPath string) error {
	resolvedSource, err := expandPath(sourcePath)
	if err != nil {
		return err
	}

	if !filepath.IsAbs(resolvedSource) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to resolve working directory: %w", err)
		}
		resolvedSource = filepath.Join(cwd, resolvedSource)
	}

	srcInfo, err := os.Stat(resolvedSource)
	if err != nil {
		return fmt.Errorf("failed to stat axdump source %s: %w", resolvedSource, err)
	}

	if err := ensureParentDir(binaryPath); err != nil {
		return fmt.Errorf("failed to create axdump binary directory: %w", err)
	}

	needBuild := true
	if binInfo, err := os.Stat(binaryPath); err == nil {
		needBuild = srcInfo.ModTime().After(binInfo.ModTime())
	}

	if !needBuild {
		return nil
	}

	out, err := exec.Command("xcrun", "swiftc", resolvedSource, "-O", "-o", binaryPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to compile axdump: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func runAXDump(binaryPath string) (*axDump, error) {
	out, err := exec.Command(binaryPath).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run axdump: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	var dump axDump
	if err := json.Unmarshal(out, &dump); err != nil {
		return nil, fmt.Errorf("failed to parse axdump JSON: %w", err)
	}

	if dump.Status != "success" {
		message := dump.Message
		if message == "" {
			message = "axdump returned non-success status"
		}
		return nil, fmt.Errorf(message)
	}

	return &dump, nil
}
