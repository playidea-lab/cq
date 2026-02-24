package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/changmin/c4-core/internal/mailbox"
	"github.com/spf13/cobra"
)

func init() {
	mailSendCmd.Flags().StringVar(&mailSendTo, "to", "", "recipient session name (required)")
	mailSendCmd.Flags().StringVar(&mailSendSubject, "subject", "", "message subject")
	mailSendCmd.Flags().StringVar(&mailSendBody, "body", "", "message body")
	mailSendCmd.Flags().StringVar(&mailSendFrom, "from", "", "sender session name (default: CQ_SESSION_NAME)")

	mailLsCmd.Flags().StringVar(&mailLsSession, "session", "", "filter by session (default: CQ_SESSION_NAME)")
	mailLsCmd.Flags().BoolVar(&mailLsUnread, "unread", false, "show only unread messages")

	mailCmd.AddCommand(mailSendCmd)
	mailCmd.AddCommand(mailLsCmd)
	mailCmd.AddCommand(mailReadCmd)
	mailCmd.AddCommand(mailRmCmd)
	rootCmd.AddCommand(mailCmd)
}

var (
	mailSendTo      string
	mailSendSubject string
	mailSendBody    string
	mailSendFrom    string
	mailLsSession   string
	mailLsUnread    bool
)

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Inter-session mail (send/ls/read/rm)",
}

var mailSendCmd = &cobra.Command{
	Use:   "send <to> <body>",
	Short: "Send a message to a session",
	Args:  cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		to := mailSendTo
		body := mailSendBody

		if len(args) >= 1 && to == "" {
			to = args[0]
		}
		if len(args) >= 2 && body == "" {
			body = args[1]
		}

		if to == "" {
			return fmt.Errorf("recipient required: use --to <session> or positional arg")
		}
		if to == "*" {
			return fmt.Errorf("--to \"*\" is reserved for broadcast; specify a session name")
		}

		from := mailSendFrom
		if from == "" {
			from = os.Getenv("CQ_SESSION_NAME")
		}

		dbPath, err := mailbox.DefaultDBPath()
		if err != nil {
			return err
		}
		ms, err := mailbox.NewMailStore(dbPath)
		if err != nil {
			return err
		}
		defer ms.Close()

		id, err := ms.Send(from, to, mailSendSubject, body, "")
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Sent message #%d to %q\n", id, to)
		return nil
	},
}

var mailLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List messages",
	RunE: func(cmd *cobra.Command, args []string) error {
		session := mailLsSession
		if session == "" {
			session = os.Getenv("CQ_SESSION_NAME")
		}

		dbPath, err := mailbox.DefaultDBPath()
		if err != nil {
			return err
		}
		ms, err := mailbox.NewMailStore(dbPath)
		if err != nil {
			return err
		}
		defer ms.Close()

		msgs, err := ms.List(session, mailLsUnread)
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			fmt.Println("No messages.")
			return nil
		}

		for _, m := range msgs {
			status := "[unread]"
			if m.ReadAt != "" {
				status = "[read]"
			}
			// Parse RFC3339 created_at for display.
			ts := m.CreatedAt
			if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
				ts = t.Local().Format("2006-01-02 15:04")
			}
			fmt.Printf("#%-3d from=%-15s → to=%-15s %s  (%s)\n",
				m.ID, m.From, m.To, status, ts)
			if m.Subject != "" {
				fmt.Printf("    Subject: %s\n", m.Subject)
			}
		}
		return nil
	},
}

var mailReadCmd = &cobra.Command{
	Use:   "read <id>",
	Short: "Read a message (marks as read)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q: %w", args[0], err)
		}

		dbPath, err := mailbox.DefaultDBPath()
		if err != nil {
			return err
		}
		ms, err := mailbox.NewMailStore(dbPath)
		if err != nil {
			return err
		}
		defer ms.Close()

		m, err := ms.Read(id)
		if err != nil {
			return err
		}

		ts := m.CreatedAt
		if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
			ts = t.Local().Format("2006-01-02 15:04")
		}
		fmt.Printf("From:    %s\n", m.From)
		fmt.Printf("To:      %s\n", m.To)
		fmt.Printf("Date:    %s\n", ts)
		if m.Subject != "" {
			fmt.Printf("Subject: %s\n", m.Subject)
		}
		fmt.Println()
		fmt.Println(m.Body)
		return nil
	},
}

var mailRmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Delete a message",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q: %w", args[0], err)
		}

		dbPath, err := mailbox.DefaultDBPath()
		if err != nil {
			return err
		}
		ms, err := mailbox.NewMailStore(dbPath)
		if err != nil {
			return err
		}
		defer ms.Close()

		if err := ms.Delete(id); err != nil {
			return err
		}
		fmt.Printf("Deleted message #%d\n", id)
		return nil
	},
}
