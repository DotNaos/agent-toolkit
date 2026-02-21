package cmd

import (
	"fmt"
	"math"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type clickCoords struct {
	X int `json:"x"`
	Y int `json:"y"`
}

var (
	clickID        string
	clickStateFile string
)

var clickCmd = &cobra.Command{
	Use:   "click",
	Short: "Click a mapped UI element by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		statePath, err := expandPath(clickStateFile)
		if err != nil {
			return err
		}

		state, err := loadState(statePath)
		if err != nil {
			return err
		}

		element, ok := findElementByID(state.Elements, clickID)
		if !ok {
			return fmt.Errorf("element with id %s not found in %s", clickID, statePath)
		}

		coords, err := boundsCenter(element.Bounds)
		if err != nil {
			return fmt.Errorf("cannot click id %s: %w", clickID, err)
		}

		if err := runCliclick(fmt.Sprintf("c:%d,%d", coords.X, coords.Y)); err != nil {
			return err
		}

		return outputJSON(map[string]any{
			"status": "success",
			"action": "click",
			"id":     strings.ToUpper(clickID),
			"coords": coords,
		})
	},
}

func init() {
	clickCmd.Flags().StringVar(&clickID, "id", "", "Mapped element id (e.g. X002)")
	clickCmd.Flags().StringVar(&clickStateFile, "state-file", defaultStatePath(), "Path to mapped UI state JSON")
	_ = clickCmd.MarkFlagRequired("id")
}

func boundsCenter(bounds *Bounds) (*clickCoords, error) {
	if bounds == nil {
		return nil, fmt.Errorf("missing bounds")
	}
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return nil, fmt.Errorf("invalid bounds width=%.2f height=%.2f", bounds.Width, bounds.Height)
	}

	cx := int(math.Round(bounds.X + (bounds.Width / 2)))
	cy := int(math.Round(bounds.Y + (bounds.Height / 2)))

	return &clickCoords{X: cx, Y: cy}, nil
}

func runCliclick(args ...string) error {
	if _, err := exec.LookPath("cliclick"); err != nil {
		return fmt.Errorf("cliclick not found in PATH; install it with `brew install cliclick`")
	}

	out, err := exec.Command("cliclick", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run cliclick %q: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
