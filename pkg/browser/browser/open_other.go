//go:build !darwin && !windows

package browser

import "github.com/pkg/browser"

func Open(url string) error {
	return browser.OpenURL(url)
}
