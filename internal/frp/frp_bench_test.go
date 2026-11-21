package frp

import "testing"

func BenchmarkSealOpenSecret(b *testing.B) {
	key := make([]byte, keySize)
	secret := "0123456789abcdef0123456789abcdef"
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		sealed, err := seal(key, secret)
		if err != nil {
			b.Fatal(err)
		}
		if opened, err := open(key, sealed); err != nil || opened != secret {
			b.Fatalf("open = %q/%v", opened, err)
		}
	}
}

func BenchmarkRenderProxyConfig(b *testing.B) {
	endpoint := &endpoint{Address: "127.0.0.1", Port: 7000, Token: "token"}
	proxy := &proxy{Name: "lab", Type: "stcp", ServicePort: 22, BindPort: 22023, Secret: "secret"}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if config := renderConfig(endpoint, proxy); len(config) == 0 {
			b.Fatal("empty config")
		}
	}
}
