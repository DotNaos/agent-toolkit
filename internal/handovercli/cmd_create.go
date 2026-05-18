package handovercli

import (
	"os"
	"path/filepath"
	"time"

	"agent-toolkit/internal/shared/cliio"
	"github.com/spf13/cobra"
)

var createOpts struct {
	title           string
	sourceSession   string
	sourceProject   string
	targetProject   string
	mode            string
	idea            string
	requestedChange string
	output          string
	stateDB         string
	acceptance      []string
	constraints     []string
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a Codex handover briefing file",
	RunE: func(cmd *cobra.Command, args []string) error {
		now := time.Now()
		sourceProject := createOpts.sourceProject
		if sourceProject == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			sourceProject = cwd
		}
		sourceProject, _ = filepath.Abs(sourceProject)
		targetProject := createOpts.targetProject
		if targetProject == "" {
			targetProject = sourceProject
		}
		targetProject, _ = filepath.Abs(targetProject)

		sourceSession := createOpts.sourceSession
		if sourceSession == "" && createOpts.stateDB != "" {
			if thread, err := LatestThreadForCWD(createOpts.stateDB, sourceProject); err == nil {
				sourceSession = thread.ID
			}
		}

		briefing := BuildBriefing(BriefingOptions{
			Title:           createOpts.title,
			SourceSession:   sourceSession,
			SourceProject:   sourceProject,
			TargetProject:   targetProject,
			Mode:            createOpts.mode,
			Idea:            createOpts.idea,
			RequestedChange: createOpts.requestedChange,
			Acceptance:      createOpts.acceptance,
			Constraints:     createOpts.constraints,
			CreatedAt:       now,
		})

		output := createOpts.output
		if output == "" {
			output = DefaultBriefingPath(targetProject, createOpts.title, now)
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(output, []byte(briefing), 0o644); err != nil {
			return err
		}

		return cliio.OutputJSON(map[string]any{
			"status":         "success",
			"action":         "create",
			"output_path":    output,
			"source_session": sourceSession,
			"source_project": sourceProject,
			"target_project": targetProject,
			"mode":           firstNonEmpty(createOpts.mode, "worktree"),
		})
	},
}

func init() {
	createOpts.stateDB = DefaultStateDBPath()
	createCmd.Flags().StringVar(&createOpts.title, "title", "", "Briefing title")
	createCmd.Flags().StringVar(&createOpts.sourceSession, "source-session", "", "Source Codex session id")
	createCmd.Flags().StringVar(&createOpts.sourceProject, "source-project", "", "Source project path (default current directory)")
	createCmd.Flags().StringVar(&createOpts.targetProject, "target-project", "", "Target project path (default source project)")
	createCmd.Flags().StringVar(&createOpts.mode, "mode", "worktree", "Implementation mode: direct, worktree, kickoff, docs, migration-skill")
	createCmd.Flags().StringVar(&createOpts.idea, "idea", "", "Idea discovered in the source session")
	createCmd.Flags().StringVar(&createOpts.requestedChange, "requested-change", "", "Concrete change requested in the target project")
	createCmd.Flags().StringVar(&createOpts.output, "output", "", "Output briefing path")
	createCmd.Flags().StringVar(&createOpts.stateDB, "state-db", createOpts.stateDB, "Codex state_5.sqlite path")
	createCmd.Flags().StringArrayVar(&createOpts.acceptance, "acceptance", nil, "Acceptance criterion (repeatable)")
	createCmd.Flags().StringArrayVar(&createOpts.constraints, "constraint", nil, "Constraint (repeatable)")
}
