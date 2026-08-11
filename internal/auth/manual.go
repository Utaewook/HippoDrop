package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"golang.org/x/oauth2"

	"hippodrop/internal/browser"
)

// RedirectURL is a loopback redirect URI used for the Authorization Code +
// PKCE flow. Google's "Desktop app" client type does not require the exact
// port to be pre-registered for loopback addresses (RFC 8252 §7.3), and no
// listener is ever bound to it — the user pastes the redirected URL back
// manually instead.
const RedirectURL = "http://localhost:8085"

// ParseAuthResponseURL extracts the OAuth "code" and "state" query
// parameters from a full redirect URL the user pastes back into the
// terminal (the address bar content after Google redirects the browser,
// whether or not that page actually loaded).
func ParseAuthResponseURL(raw string) (code, state string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", fmt.Errorf("could not parse the pasted URL: %w", err)
	}
	q := u.Query()
	code = q.Get("code")
	state = q.Get("state")
	if code == "" {
		return "", "", errors.New("no 'code' parameter found in the pasted URL")
	}
	return code, state, nil
}

// RunManualFlow drives an Authorization Code + PKCE exchange without a
// local callback listener. The redirect URI is never actually served —
// Google still redirects the browser to it after consent, and the user
// copies that address bar content back into the terminal. This works
// identically on a local machine, inside a Docker container, or over SSH,
// since it never depends on the browser being able to reach the CLI process.
func RunManualFlow(ctx context.Context, gConfig *oauth2.Config) (*oauth2.Token, error) {
	codeVerifier := GenerateCodeVerifier()
	codeChallenge := GenerateCodeChallenge(codeVerifier)
	state := GenerateState()

	authURL := gConfig.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	fmt.Println()
	fmt.Println("\033[1;36mGoogle Drive Authorization\033[0m")
	fmt.Println("\033[36m────────────────────────────────────────────────────────────\033[0m")
	fmt.Println("1. 다음 URL을 웹 브라우저에서 열어 로그인 및 동의를 완료해주세요:")
	fmt.Printf("   \033[32;4m%s\033[0m\n\n", authURL)
	fmt.Println("2. 동의 후 이동하는 페이지에서 \"사이트에 연결할 수 없음\"이 떠도 정상입니다.")
	fmt.Println("   그 화면의 주소창에 표시된 전체 URL을 복사해주세요.")
	fmt.Println("\033[36m────────────────────────────────────────────────────────────\033[0m")

	_ = browser.Open(authURL)

	fmt.Print("\n3. 복사한 전체 URL을 여기에 붙여넣고 Enter를 눌러주세요: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	code, respState, err := ParseAuthResponseURL(line)
	if err != nil {
		return nil, err
	}
	if respState != state {
		return nil, errors.New("state mismatch: possible CSRF, authentication aborted")
	}

	tok, err := gConfig.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	return tok, nil
}
