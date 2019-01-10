package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/pkg/errors"
	"github.com/pkg/profile"
	"github.com/urfave/cli"
	"go.uber.org/zap"

	"fb2converter/commands"
	"fb2converter/config"
	"fb2converter/misc"
	"fb2converter/state"
)

type appWrapper struct {
	log           *zap.Logger
	stdlogRestore func()
	prof          interface{ Stop() }
}

func (w *appWrapper) beforeRun(c *cli.Context) error {

	if c.NArg() == 0 {
		return nil
	}

	const (
		errPrefix = "\n*** ERROR ***\n\npreparing: "
		errCode   = 1
	)
	var err error

	// Process global options

	env := c.GlobalGeneric(state.FlagName).(*state.LocalEnv)
	env.Debug = c.GlobalBool("debug")
	env.Mhl = c.GlobalBool("mhl")

	// Prepare configuration
	fconfig := c.GlobalString("config")
	if env.Cfg, err = config.BuildConfig(fconfig); err != nil {
		return cli.NewExitError(errors.Wrapf(err, "%sunable to build configuration", errPrefix), errCode)
	}

	// Prepare logs
	env.Log, err = env.Cfg.PrepareLog()
	if err != nil {
		return cli.NewExitError(errors.Wrapf(err, "%sunable to create logs", errPrefix), errCode)
	}

	w.log = env.Log
	w.stdlogRestore = zap.RedirectStdLog(env.Log)

	// We may want to do some profiling
	if p := c.String("cpuprofile"); len(p) > 0 {
		w.prof = profile.Start(profile.CPUProfile, profile.ProfilePath(p))
	} else if p := c.String("memprofile"); len(p) > 0 {
		w.prof = profile.Start(profile.MemProfile, profile.ProfilePath(p))
	} else if p := c.String("blkprofile"); len(p) > 0 {
		w.prof = profile.Start(profile.BlockProfile, profile.ProfilePath(p))
	} else if p := c.String("traceprofile"); len(p) > 0 {
		w.prof = profile.Start(profile.TraceProfile, profile.ProfilePath(p))
	} else if p := c.String("mutexprofile"); len(p) > 0 {
		w.prof = profile.Start(profile.MutexProfile, profile.ProfilePath(p))
	}

	w.log.Debug("Program started", zap.Strings("args", os.Args), zap.String("ver", misc.GetVersion()+" ("+runtime.Version()+") : "+LastGitCommit))
	if len(fconfig) == 0 {
		w.log.Debug("Using defaults (no configuration file)")
	}
	return nil
}

func (w *appWrapper) afterRun(c *cli.Context) error {

	if w.prof != nil {
		w.prof.Stop()
	}

	if w.log != nil {

		w.log.Debug("Program ended", zap.Strings("parsed args", c.Args()))

		w.stdlogRestore()
		w.log.Sync()
	}
	return nil
}

// LastGitCommit is used during build to inject git sha
var LastGitCommit string

func main() {

	cli.OsExiter = func(int) { /* do nothing, we want afterRun to execute */ }

	app := cli.NewApp()

	app.Name = "fb2converter"
	app.Usage = "fb2 conversion engine"
	app.Version = misc.GetVersion() + " (" + runtime.Version() + ") : " + LastGitCommit

	var wrap appWrapper
	app.Before = wrap.beforeRun
	app.After = wrap.afterRun

	app.Flags = []cli.Flag{
		// only one profile could be enables at a time - this is enforced by beforeRun
		cli.StringFlag{Name: "cpuprofile", Hidden: true, Usage: "write cpu profile to `PATH`"},
		cli.StringFlag{Name: "memprofile", Hidden: true, Usage: "write memory profile to `PATH`"},
		cli.StringFlag{Name: "blkprofile", Hidden: true, Usage: "write block profile to `PATH`"},
		cli.StringFlag{Name: "traceprofile", Hidden: true, Usage: "write trace profile to `PATH`"},
		cli.StringFlag{Name: "mutexprofile", Hidden: true, Usage: "write mutex profile to `PATH`"},

		cli.GenericFlag{Name: state.FlagName, Hidden: true, Usage: "--internal--", Value: state.NewLocalEnv()},
		cli.BoolFlag{Name: "mhl", Hidden: true, Usage: "--internal--"},

		cli.StringFlag{Name: "config, c", Usage: "load configuration from `FILE` (YAML, TOML or JSON). if FILE is \"-\" JSON will be expected from STDIN"},
		cli.BoolFlag{Name: "debug, d", Usage: "leave behind various artifacts for debugging (do not delete intermediate results)"},
	}

	app.Commands = []cli.Command{
		{
			Name:   "convert",
			Usage:  "Converts file(s) to specified format",
			Action: commands.Convert,
			Flags: []cli.Flag{
				cli.StringFlag{Name: "to", Value: "epub", Usage: "conversion output `TYPE` (supported types: epub, azw3, mobi)"},
				cli.BoolFlag{Name: "nodirs", Usage: "when producing output do not keep input directory structure"},
				cli.BoolFlag{Name: "stk", Usage: "send converted file to kindle (mobi only)"},
				cli.BoolFlag{Name: "ow", Usage: "continue even if destination exits, overwrite files"},
			},
			ArgsUsage: "SOURCE [DESTINATION]",
			CustomHelpTemplate: fmt.Sprintf(`%sSOURCE:
    path to fb2 file(s) to process, following formats are supported:
        path to a file: [path]file.fb2
        path to a directory: [path]directory - recursively process all files under directory (symbolic links are not followed)
        path to archive with path inside archive to a particular fb2 file: [path]archive.zip[archive path]/file.fb2
        path to archive with path inside archive: [path]archive.zip[archive path] - recursively process all fb2 files under archive path

    When working on archive recursively only fb2 files will be considered, processing of archives inside archives is not supported.

DESTINATION:
    always a path, output file name(s) and extension will be derived from other parameters
    if absent - current working directory
`, cli.CommandHelpTemplate),
		},
		{
			Name:      "dumpconfig",
			Usage:     "Dumps active configuration (JSON)",
			Action:    commands.DumpConfig,
			ArgsUsage: "DESTINATION",
			CustomHelpTemplate: fmt.Sprintf(`%s
DESTINATION:
	file name to write configuration to, if absent - STDOUT

Produces file with actual configuration values to be used by the program. To see configuration after parsing but before anything else use --debug option.
`, cli.CommandHelpTemplate),
		},
		{
			Name:      "export",
			Usage:     "Exports built-in resources for customization",
			Action:    commands.ExportResources,
			ArgsUsage: "DESTINATION",
			CustomHelpTemplate: fmt.Sprintf(`%s
DESTINATION:
	existing path to export resources to, must be present

Exports built-in resources (example configuration, style sheets, fonts, etc.) for customization. With --debug option will export all built-in resources, even non-customizable.
`, cli.CommandHelpTemplate),
		},
		{
			Name:      "synccovers",
			Usage:     "Extracts thumbnails from documents (Kindle only!)",
			Action:    commands.SyncCovers,
			ArgsUsage: "SOURCE",
			CustomHelpTemplate: fmt.Sprintf(`%s
SOURCE:
	directory on mounted device to look for books

Synchronizes kindle thumbnails with actual documents.
`, cli.CommandHelpTemplate),
		},
	}

	if err := app.Run(os.Args); err != nil {
		if wrap.log != nil {
			// wrap.log.Error("unable to continue", zap.Error(err))
			wrap.log.Sync()
		}
		os.Exit(1)
	}
}
