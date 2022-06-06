package ssmclient

import (
	"io"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/brunetto/ssm-session-client/datachannel"
	"github.com/pkg/errors"
)

// ShellSession starts a shell session with the instance specified in the target parameter.  The
// client.ConfigProvider parameter will be used to call the AWS SSM StartSession API, which is used
// as part of establishing the websocket communication channel.  A vararg slice of io.Readers can
// be provided to send data to the instance before handing control of the terminal to the user.
func ShellSession(cfg aws.Config, target string, initCmd ...io.Reader) error {
	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, &ssm.StartSessionInput{Target: aws.String(target)}); err != nil {
		return err
	}
	defer c.Close()

	// do platform-specific setup ... signal handling, stdin modification, etc...
	if err := initialize(c); err != nil {
		return err
	}
	defer cleanup() //nolint:errcheck // platform-specific cleanup, not called if terminated by a signal

	errCh := make(chan error, 5)
	go func() {
		if _, err := io.Copy(c, os.Stdin); err != nil {
			errCh <- err
		}
	}()

	for _, cmd := range initCmd {
		_, _ = io.Copy(c, cmd)
	}

	if _, err := io.Copy(os.Stdout, c); err != nil {
		if !errors.Is(err, io.EOF) {
			errCh <- err
		}
		close(errCh)
	}

	return <-errCh
}

func ShellSessionV2(cfg aws.Config, target string, r io.Reader, w io.Writer, initCmd ...io.Reader) error {
	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, &ssm.StartSessionInput{Target: aws.String(target)}); err != nil {
		return err
	}
	defer c.Close()

	// do platform-specific setup ... signal handling, stdin modification, etc...
	if err := initialize(c); err != nil {
		return err
	}
	defer cleanup() //nolint:errcheck // platform-specific cleanup, not called if terminated by a signal

	errCh := make(chan error, 5)
	go func() {
		if _, err := io.Copy(c, r); err != nil {
			errCh <- err
		}
	}()

	for _, cmd := range initCmd {
		_, _ = io.Copy(c, cmd)
	}

	if _, err := io.Copy(w, c); err != nil {
		if !errors.Is(err, io.EOF) {
			errCh <- err
		}

		close(errCh)
	}

	return <-errCh
}

func updateTermSize(c datachannel.DataChannel) error {
	rows, cols, err := getWinSize()
	if err != nil {
		// make sure we set some default terminal size with contrived values
		cols = 132
		rows = 45
		log.Printf("Could not get size of the terminal: %s, using width %d height %d\n", err, cols, rows)
	}

	return c.SetTerminalSize(rows, cols)
}
