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

// RunSudo runs cmd under sudo, feeding the password on stdin. A plain "sudo" over
// a non-interactive SSH session has no TTY to prompt on and fails immediately;
// -S reads from stdin instead and -p '' keeps the prompt out of the output.
// Harmless when the account has passwordless sudo — the stdin is simply ignored.
func (c *SSHClient) RunSudo(cmd, password string) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	sess.Stdin = strings.NewReader(password + "\n")
	out, err := sess.CombinedOutput("sudo -S -p '' " + cmd)
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

// supportedDistros are the distributions the provisioning playbook knows how to
// install packages on. Keep in sync with the ansible_os_family branches in the
// docker role.
var supportedDistros = []string{"ubuntu", "arch"}

// CheckSupportedOS verifies the target runs a distribution the playbook supports.
func CheckSupportedOS(host, username, password string) error {
	c, err := Dial(host, username, password)
	if err != nil {
		return err
	}
	defer c.Close()

	out, err := c.Run("lsb_release -is 2>/dev/null || cat /etc/os-release | grep ^ID=")
	if err != nil {
		return fmt.Errorf("os check failed: %w", err)
	}
	got := strings.ToLower(out)
	for _, distro := range supportedDistros {
		if strings.Contains(got, distro) {
			return nil
		}
	}
	return fmt.Errorf("unsupported distro, expected one of %s (got: %s)",
		strings.Join(supportedDistros, ", "), out)
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

func DisablePasswordAuth(host, username, privateKey, sudoPass string) error {
	c, err := DialWithKey(host, username, privateKey)
	if err != nil {
		return err
	}
	defer c.Close()

	cmds := []string{
		`sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config`,
		`sed -i 's/^#*ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' /etc/ssh/sshd_config`,
	}
	for _, cmd := range cmds {
		// Include the command output: sudo reports why it refused on stderr, and
		// swallowing it leaves nothing to debug but an exit status.
		if out, err := c.RunSudo(cmd, sudoPass); err != nil {
			return fmt.Errorf("disable password auth (%q): %w: %s", cmd, err, out)
		}
	}

	// The unit is "sshd" on Arch and "ssh" on Debian/Ubuntu; try both.
	var restartErr error
	for _, unit := range []string{"sshd", "ssh"} {
		out, err := c.RunSudo("systemctl restart "+unit, sudoPass)
		if err == nil {
			return nil
		}
		restartErr = fmt.Errorf("restart %s: %w: %s", unit, err, out)
	}
	return fmt.Errorf("disable password auth: %w", restartErr)
}
