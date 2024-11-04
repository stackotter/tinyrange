package browser

import (
	"os"
	"os/exec"
)

func Open(url string) error {
	cmd := exec.Command("cmd", "/c", "start", "msedge", "--new-window", "--app="+url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
