package cli

import (
	"fmt"
	"os"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdOrg dispatches `ppz org <subverb>`:
//
//	ppz org list                 list orgs the logged-in user belongs to
//	ppz org switch <name>        switch the daemon's active org
//	ppz org create <name>        create a new org (caller becomes owner)
//	ppz org invite <username>    invite a user to the current org (owner-only)
func cmdOrg(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz org {list|switch|create|invite} ...")
		os.Exit(2)
	}
	switch args[0] {
	case "list", "ls":
		return cmdOrgList(args[1:])
	case "switch":
		return cmdOrgSwitch(args[1:])
	case "create":
		return cmdOrgCreate(args[1:])
	case "invite":
		return cmdOrgInvite(args[1:])
	}
	fmt.Fprintf(os.Stderr, "ppz org: unknown subcommand %q\n", args[0])
	os.Exit(2)
	return nil
}

func cmdOrgList(args []string) error {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz org list")
		os.Exit(2)
	}
	var reply cliproto.ListOrgsReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCOrgList, struct{}{}, &reply); err != nil {
		return err
	}
	for _, o := range reply.Orgs {
		role := o.Role
		if role == "" {
			role = "member"
		}
		fmt.Fprintf(os.Stdout, "%s\t%s\n", o.Name, role)
	}
	return nil
}

func cmdOrgSwitch(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz org switch <name>")
		os.Exit(2)
	}
	var reply cliproto.OrgSwitchReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCOrgSwitch,
		cliproto.OrgSwitchRequest{Name: args[0]}, &reply); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "switched org=%s id=%s\n", reply.OrgName, reply.OrgID)
	return nil
}

func cmdOrgCreate(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz org create <name>")
		os.Exit(2)
	}
	var reply cliproto.CreateOrgReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCOrgCreate,
		cliproto.OrgCreateRequest{Name: args[0]}, &reply); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "created org=%s id=%s\n", reply.Name, reply.ID)
	return nil
}

func cmdOrgInvite(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz org invite <username>")
		os.Exit(2)
	}
	var reply cliproto.CreateInviteReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCOrgInvite,
		cliproto.OrgInviteRequest{Username: args[0]}, &reply); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "invited username=%s org=%s id=%s\n",
		reply.Invite.InviteeUsername, reply.Invite.OrganisationName, reply.Invite.ID)
	return nil
}
