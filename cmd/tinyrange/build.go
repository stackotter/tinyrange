package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/tinyrange/tinyrange/pkg/common"
)

var (
	buildOutput string
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a single definition",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("please specify a definition")
		}

		db, err := newDb()
		if err != nil {
			return err
		}

		macroCtx := db.NewMacroContext()

		macro, err := db.GetMacroByShorthand(macroCtx, args[0])
		if err != nil {
			return err
		}

		ret, err := macro.Call(macroCtx)
		if err != nil {
			return err
		}

		if ret == nil {
			return nil
		}

		if def, ok := ret.(common.BuildDefinition); ok {
			f, err := db.Build(db.NewBuildContext(def), def, common.BuildOptions{
				AlwaysRebuild: true,
			})
			if err != nil {
				slog.Error("fatal", "err", err)
				os.Exit(1)
			}

			if buildOutput != "" {
				fh, err := f.Open()
				if err != nil {
					return err
				}
				defer fh.Close()

				out, err := os.Create(buildOutput)
				if err != nil {
					return err
				}
				defer out.Close()

				if _, err := io.Copy(out, fh); err != nil {
					return err
				}
			}

			return nil
		} else {
			return fmt.Errorf("could not convert %T to BuildDefinition", ret)
		}
	},
}

func init() {
	buildCmd.PersistentFlags().StringVarP(&buildOutput, "output", "o", "", "if specified then copy the build output to a local file at path")
	rootCmd.AddCommand(buildCmd)
}
