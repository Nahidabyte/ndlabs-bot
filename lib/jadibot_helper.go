package lib

import "go.mau.fi/whatsmeow"

func (jm *JadibotManager) IsJadibot(client *whatsmeow.Client) bool {
	if jm == nil {
		return false
	}
	jm.Mu.RLock()
	defer jm.Mu.RUnlock()
	for _, c := range jm.Clients {
		if c == client {
			return true
		}
	}
	return false
}
