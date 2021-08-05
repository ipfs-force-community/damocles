package internal

import (
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

type PrintHelpErr struct {
	Ctx *cli.Context
	Err error
}

func (e *PrintHelpErr) Error() string {
	return e.Err.Error()
}

func (e *PrintHelpErr) Unwrap() error {
	return e.Err
}

func (e *PrintHelpErr) Is(o error) bool {
	_, ok := o.(*PrintHelpErr)
	return ok
}

func ShowHelp(cctx *cli.Context, err error) error {
	return &PrintHelpErr{
		Ctx: cctx,
		Err: err,
	}
}

func ShowHelpf(cctx *cli.Context, format string, args ...interface{}) error {
	return ShowHelp(cctx, fmt.Errorf(format, args...))
}

func RunApp(app *cli.App) {
	if err := app.Run(os.Args); err != nil {
		Log.Warnf("%+v", err)
		var phe *PrintHelpErr
		if errors.As(err, &phe) {
			_ = cli.ShowCommandHelp(phe.Ctx, phe.Ctx.Command.Name)
		}
		os.Exit(1)
	}
}
