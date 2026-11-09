//go:build !windows

package strength

import "errors"

// noTokens stands in where there is no Windows Credential Manager. Terran CC
// runs on Windows; this keeps other platforms compiling, and refuses to keep
// a token rather than put one in a file.
type noTokens struct{}

// SystemTokens is the token store for this machine's signed-in user.
func SystemTokens(home string) TokenStore { return noTokens{} }

var errNoStore = errors.New("storing the 99-Strength token needs Windows Credential Manager")

func (noTokens) Get() (string, error) { return "", ErrNoToken }
func (noTokens) Set(string) error     { return errNoStore }
func (noTokens) Delete() error        { return nil }
func (noTokens) Where() string        { return "nowhere (Windows only)" }
