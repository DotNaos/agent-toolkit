package uilloopcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	typeID        string
	typeText      string
	typeSubmit    bool
	typeDelayMs   int
	typeStateFile string
)

var typeCmd = &cobra.Command{
	Use:   "type",
	Short: "Focus an element by ID and type text",
	RunE: func(cmd *cobra.Command, args []string) error {
		statePath, err := expandPath(typeStateFile)
		if err != nil {
			return err
		}

		state, err := loadState(statePath)
		if err != nil {
			return err
		}

		element, ok := findElementByID(state.Elements, typeID)
		if !ok {
			return fmt.Errorf("element with id %s not found in %s", typeID, statePath)
		}

		coords, err := boundsCenter(element.Bounds)
		if err != nil {
			return fmt.Errorf("cannot type into id %s: %w", typeID, err)
		}

		if err := runCliclick(fmt.Sprintf("c:%d,%d", coords.X, coords.Y)); err != nil {
			return err
		}

		if typeDelayMs > 0 {
			time.Sleep(time.Duration(typeDelayMs) * time.Millisecond)
		}

		if err := runCliclick(fmt.Sprintf("t:%s", typeText)); err != nil {
			return err
		}

		if typeSubmit {
			if err := runCliclick("kp:enter"); err != nil {
				return err
			}
		}

		return outputJSON(map[string]any{
			"status":    "success",
			"action":    "type",
			"id":        strings.ToUpper(typeID),
			"coords":    coords,
			"text":      typeText,
			"submitted": typeSubmit,
		})
	},
}

func init() {
	typeCmd.Flags().StringVar(&typeID, "id", "", "Mapped element id (e.g. X003)")
	typeCmd.Flags().StringVar(&typeText, "text", "", "Text to type")
	typeCmd.Flags().BoolVar(&typeSubmit, "submit", false, "Press Enter after typing")
	typeCmd.Flags().IntVar(&typeDelayMs, "delay-ms", 100, "Delay in milliseconds between click and typing")
	typeCmd.Flags().StringVar(&typeStateFile, "state-file", defaultStatePath(), "Path to mapped UI state JSON")
	_ = typeCmd.MarkFlagRequired("id")
	_ = typeCmd.MarkFlagRequired("text")
}
