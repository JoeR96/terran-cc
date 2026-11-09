package strength

import (
	"errors"

	"github.com/danieljoos/wincred"
)

// winTokens keeps the token as a generic credential in Windows Credential
// Manager (Control Panel > Credential Manager > Windows Credentials).
type winTokens struct{ target string }

// SystemTokens is the token store for this machine's signed-in user.
func SystemTokens(home string) TokenStore { return winTokens{target: credentialTarget(home)} }

func (w winTokens) Get() (string, error) {
	c, err := wincred.GetGenericCredential(w.target)
	if errors.Is(err, wincred.ErrElementNotFound) {
		return "", ErrNoToken
	}
	if err != nil {
		return "", err
	}
	if len(c.CredentialBlob) == 0 {
		return "", ErrNoToken
	}
	return string(c.CredentialBlob), nil
}

func (w winTokens) Set(token string) error {
	c := wincred.NewGenericCredential(w.target)
	c.UserName = "terran-cc"
	c.Comment = "99-Strength personal API token used by Terran CC"
	c.CredentialBlob = []byte(token)
	c.Persist = wincred.PersistLocalMachine
	return c.Write()
}

func (w winTokens) Delete() error {
	c, err := wincred.GetGenericCredential(w.target)
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return c.Delete()
}

func (w winTokens) Where() string { return "Windows Credential Manager (" + w.target + ")" }
