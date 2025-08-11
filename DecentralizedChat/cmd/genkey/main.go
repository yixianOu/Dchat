package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/nacl/box"
)

func main() {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}
	privB64 := base64.StdEncoding.EncodeToString(priv[:])
	pubB64 := base64.StdEncoding.EncodeToString(pub[:])
	fmt.Println("PRIVATE_BASE64=" + privB64)
	fmt.Println("PUBLIC_BASE64=" + pubB64)
	fmt.Fprintln(os.Stderr, "请安全保存私钥，仅将 PUBLIC_BASE64 提供给好友")
}
