package main

import (
    "fmt"
    "os"
    "os/exec"
    "os/signal"
    "syscall"
)

func main() {
    cmd := exec.Command("go", "test", "./...", "-coverprofile=coverage.out")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Env = os.Environ()
    if err := cmd.Start(); err != nil {
        fmt.Fprintf(os.Stderr, "failed to start tests: %v\n", err)
        os.Exit(1)
    }

    sigc := make(chan os.Signal, 1)
    signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigc
        _ = cmd.Process.Signal(os.Interrupt)
    }()

    if err := cmd.Wait(); err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            os.Exit(exitErr.ExitCode())
        }
        fmt.Fprintf(os.Stderr, "tests failed: %v\n", err)
        os.Exit(1)
    }
}
