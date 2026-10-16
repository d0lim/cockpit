package httpport

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const planted = "PLANTED-token-7f3a9c"

var httpMutations = []porttest.Mutation{
	{"허용 오리진 밖 거부", "origin 검사를 지운다", "빨강: 다른 호스트·IP·https 요청이 통과한다", "Test허용된_서비스_이름_요청만_통과하고_다른_호스트와_IP와_https와_상대_경로는_PORT_DENIED_다"},
	{"거부된 요청은 세지 않음", "검사 전에 Sent 를 올린다", "빨강: 거부된 요청 뒤 Sent 가 0 이 아니다", "Test허용된_서비스_이름_요청만_통과하고_다른_호스트와_IP와_https와_상대_경로는_PORT_DENIED_다"},
	{"리다이렉트 거부", "3xx 검사를 지운다", "빨강: 302 응답이 Response 로 돌아온다", "Test리다이렉트_응답은_PORT_DENIED_이고_보낸_것으로_센다"},
	{"시간 상한", "WithTimeout 을 지운다", "빨강: 느린 서버에서 시험이 멈춘다", "Test시간_상한을_넘기면_PORT_TIMEOUT_이고_상한_안_응답은_그대로다"},
	{"본문 상한", "LimitReader 를 지운다", "빨강: 상한을 넘는 본문이 그대로 돌아온다", "Test본문_상한을_넘기면_PORT_LIMIT_이고_상한_안_본문은_잘리지_않는다"},
	{"오류 문장 마스킹", "거부 문장을 mask 없이 만든다", "빨강: 경로에 심은 값이 오류 문장에 보인다", "Test거부와_실패_문장은_마스커를_지나_심은_값이_없고_쿼리를_싣지_않는다"},
	{"프록시 환경 변수 무시", "Transport.Proxy 를 ProxyFromEnvironment 로 바꾼다", "빨강: HTTP_PROXY 가 걸린 환경에서 요청이 프록시로 간다", "Test허용된_서비스_이름_요청만_통과하고_다른_호스트와_IP와_https와_상대_경로는_PORT_DENIED_다"},
}

func server(t *testing.T) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/echo":
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("X-Echo-Content-Type", r.Header.Get("Content-Type"))
			w.Header().Set("X-Echo-Auth", r.Header.Get("Authorization"))
			w.Header().Add("X-Multi", "a")
			w.Header().Add("X-Multi", "b")
			w.WriteHeader(201)
			w.Write(body)
		case "/redirect":
			w.Header().Set("Location", "/echo")
			w.WriteHeader(302)
		case "/slow":
			time.Sleep(300 * time.Millisecond)
			w.WriteHeader(200)
		case "/big":
			w.Write([]byte(strings.Repeat("x", 1025)))
		case "/exact":
			w.Write([]byte(strings.Repeat("y", 1024)))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	c, err := New(Options{
		Allowed: []string{"http://app:8080", "http://mock:9090"},
		Timeout: 100 * time.Millisecond,
		MaxBody: 1024,
		Mask:    func(s string) string { return strings.ReplaceAll(s, planted, "****") },
		Dial: func(ctx context.Context, network, dialed string) (net.Conn, error) {
			if dialed != "app:8080" && dialed != "mock:9090" {
				return nil, errors.New("unexpected dial " + dialed)
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func codeOf(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func Test허용된_서비스_이름_요청만_통과하고_다른_호스트와_IP와_https와_상대_경로는_PORT_DENIED_다(t *testing.T) {
	c, _ := server(t)
	res, err := c.Send(context.Background(), Request{Method: "POST", URL: "http://app:8080/echo?q=1", Headers: map[string]string{"Authorization": "Bearer x"}, JSON: map[string]any{"a": 1}})
	if err != nil || res.Status != 201 || res.Body != `{"a":1}` || res.Headers["x-echo-content-type"] != "application/json" || res.Headers["x-echo-auth"] != "Bearer x" || res.Headers["x-multi"] != "a, b" || res.Method != "POST" || res.URL != "http://app:8080/echo?q=1" {
		t.Fatalf("허용 요청: %+v %v", res, err)
	}
	if n := c.Counter(); n.Sent != 1 || n.LastStatus != 201 {
		t.Fatalf("카운터: %+v", n)
	}
	c.ResetCounter()
	denied := []string{"http://evil:8080/echo", "http://127.0.0.1:1/echo", "https://app:8080/echo", "/echo", "http://app:9999/echo", "http://app/echo"}
	for _, u := range denied {
		if _, err := c.Send(context.Background(), Request{Method: "GET", URL: u}); codeOf(err) != engine.PortDenied {
			t.Fatalf("%s: %v", u, err)
		}
	}
	if n := c.Counter(); n.Sent != 0 || n.LastStatus != 0 {
		t.Fatalf("거부 뒤 카운터: %+v (거부 %d건)", n, len(denied))
	}
	if _, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://mock:9090/echo"}); err != nil {
		t.Fatalf("둘째 허용 오리진: %v", err)
	}
}

func Test리다이렉트_응답은_PORT_DENIED_이고_보낸_것으로_센다(t *testing.T) {
	c, _ := server(t)
	_, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/redirect"})
	if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), "리다이렉트") {
		t.Fatalf("%v", err)
	}
	if n := c.Counter(); n.Sent != 1 || n.LastStatus != 302 {
		t.Fatalf("%+v", n)
	}
}

func Test시간_상한을_넘기면_PORT_TIMEOUT_이고_상한_안_응답은_그대로다(t *testing.T) {
	c, _ := server(t)
	_, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/slow"})
	if codeOf(err) != engine.PortTimeout || !strings.Contains(err.Error(), "100ms") || !strings.Contains(err.Error(), "GET /slow") {
		t.Fatalf("%v", err)
	}
	res, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/slow", TimeoutMs: 2000})
	if err != nil || res.Status != 200 {
		t.Fatalf("요청별 상한: %+v %v", res, err)
	}
	if n := c.Counter(); n.Sent != 2 {
		t.Fatalf("%+v", n)
	}
}

func Test본문_상한을_넘기면_PORT_LIMIT_이고_상한_안_본문은_잘리지_않는다(t *testing.T) {
	c, _ := server(t)
	if _, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/big"}); codeOf(err) != engine.PortLimit {
		t.Fatalf("%v", err)
	}
	res, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/exact"})
	if err != nil || len(res.Body) != 1024 {
		t.Fatalf("%d %v", len(res.Body), err)
	}
}

func Test거부와_실패_문장은_마스커를_지나_심은_값이_없고_쿼리를_싣지_않는다(t *testing.T) {
	c, srv := server(t)
	_, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://evil:8080/" + planted + "?token=" + planted})
	if err == nil || strings.Contains(err.Error(), planted) || !strings.Contains(err.Error(), "****") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("%v", err)
	}
	srv.Close()
	_, err = c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/" + planted + "?token=" + planted})
	if codeOf(err) != engine.PortUnavailable || strings.Contains(err.Error(), planted) || strings.Contains(err.Error(), "token=") || !strings.Contains(err.Error(), "GET /****") {
		t.Fatalf("%v", err)
	}
	if _, err := New(Options{Allowed: []string{"app:8080"}, Mask: ports.Identity}); err == nil {
		t.Fatal("스킴 없는 허용 오리진")
	}
}

func TestHTTP_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	porttest.AssertMutationRecord(t, "http_test.go", httpMutations)
}
