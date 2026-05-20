package cli

import (
	"os"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

func effectiveCurrentHandle() (string, error) {
	if h := os.Getenv("PPZ_CURRENT_HANDLE"); h != "" {
		return h, nil
	}
	var st cliproto.StatusReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCStatus,
		cliproto.StatusRequest{Session: sessionID(), AncestorPIDs: ancestorPIDs()}, &st); err != nil {
		return "", err
	}
	if st.Current == "" {
		return "", cliproto.New(cliproto.ENoCurrentSource)
	}
	return st.Current, nil
}
