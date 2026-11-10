package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/client"

	"docker-tool/internal/screen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "anchors:", err)
		os.Exit(1)
	}
}

func run() error {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	defer cli.Close()

	app, err := screen.NewApp(cli)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseAllMotion()).Run()
	return err
}
