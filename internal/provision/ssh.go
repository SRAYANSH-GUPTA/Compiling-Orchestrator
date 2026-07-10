package provision

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	client *ssh.Client
}

func Dial(host, username, password string) (*SSHClient, error) {
	cfg := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	addr := net.JoinHostPort(host, "22")
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return &SSHClient{client: client}, nil
}

func DialWithKey(host, username, privateKey string) (*SSHClient, error) {
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	cfg := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(host, "22"), cfg)
	if err != nil {
		return nil, err
	}
	return &SSHClient{client: client}, nil
}

func (c *SSHClient) Close() {
	c.client.Close()
}

func (c *SSHClient) Run(cmd string) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return strings.TrimSpace(string(out)), err
}

func TestConnection(host, username, password string) error {
	c, err := Dial(host, username, password)
	if err != nil {
		return err
	}
	defer c.Close()

	out, err := c.Run("uname -a")
	if err != nil {
		return fmt.Errorf("test command failed: %w", err)
	}
	if !strings.Contains(strings.ToLower(out), "linux") {
		return fmt.Errorf("target does not appear to be a Linux machine")
	}
	return nil
}

func CheckUbuntu(host, username, password string) error {
	c, err := Dial(host, username, password)
	if err != nil {
		return err
	}
	defer c.Close()

	out, err := c.Run("lsb_release -is 2>/dev/null || cat /etc/os-release | grep ^ID=")
	if err != nil {
		return fmt.Errorf("os check failed: %w", err)
	}
	if !strings.Contains(strings.ToLower(out), "ubuntu") {
		return fmt.Errorf("target is not Ubuntu (got: %s)", out)
	}
	return nil
}

func InstallSSHKey(host, username, password, pubKey string) error {
	c, err := Dial(host, username, password)
	if err != nil {
		return err
	}
	defer c.Close()

	cmds := []string{
		"mkdir -p ~/.ssh && chmod 700 ~/.ssh",
		fmt.Sprintf("echo '%s' >> ~/.ssh/authorized_keys", pubKey),
		"chmod 600 ~/.ssh/authorized_keys",
	}
	for _, cmd := range cmds {
		if _, err := c.Run(cmd); err != nil {
			return fmt.Errorf("install ssh key (%q): %w", cmd, err)
		}
	}
	return nil
}

func DisablePasswordAuth(host, username, privateKey string) error {
	c, err := DialWithKey(host, username, privateKey)
	if err != nil {
		return err
	}
	defer c.Close()

	cmds := []string{
		`sudo sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config`,
		`sudo sed -i 's/^#*ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' /etc/ssh/sshd_config`,
		`sudo systemctl restart sshd || sudo service ssh restart`,
	}
	for _, cmd := range cmds {
		if _, err := c.Run(cmd); err != nil {
			return fmt.Errorf("disable password auth (%q): %w", cmd, err)
		}
	}
	return nil
}
