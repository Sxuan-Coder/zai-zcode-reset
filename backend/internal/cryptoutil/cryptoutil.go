// Package cryptoutil 提供密码哈希（PBKDF2-HMAC-SHA256）与上游令牌静态加密（AES-256-GCM）。
// PBKDF2 按 RFC 8018 手写实现（仅用标准库 hmac/sha256），避免对标准库版本签名的依赖。
package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const (
	pbkdf2Iterations = 600_000
	pbkdf2SaltBytes  = 16
	pbkdf2KeyBytes   = 32
	encPrefix        = "enc:v1:"
)

// pbkdf2Key 是 RFC 8018 5.2 的 PBKDF2，PRF 固定为 HMAC-SHA256。
func pbkdf2Key(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	blocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	buf := make([]byte, 4)
	u := make([]byte, hashLen)
	for block := 1; block <= blocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		prf.Write(buf)
		out = prf.Sum(out)
		t := out[len(out)-hashLen:]
		copy(u, t)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = u[:0]
			u = prf.Sum(u)
			for i := range u {
				t[i] ^= u[i]
			}
		}
	}
	return out[:keyLen]
}

// HashPassword 返回格式：pbkdf2-sha256$<iter>$<salt-hex>$<hash-hex>。
func HashPassword(password string) (string, error) {
	salt := make([]byte, pbkdf2SaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2Key([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyBytes)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iterations,
		hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

// VerifyPassword 常量时间比较；无法解析的格式直接拒绝。
func VerifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 || iter > 10_000_000 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil || len(want) == 0 || len(want) > 128 {
		return false
	}
	got := pbkdf2Key([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

type SecretBox struct{ aead cipher.AEAD }

func NewSecretBox(masterKey []byte) (*SecretBox, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("主密钥必须是 32 字节")
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

// Encrypt 输出 enc:v1:<iv>.<tag>.<ciphertext>（base64url 分段，与 ZCode 凭据格式一致）。
func (b *SecretBox) Encrypt(plain string) (string, error) {
	iv := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nil, iv, []byte(plain), nil)
	// GCM Seal 输出 = 密文 || tag，按 iv.tag.cipher 顺序拆开存放。
	tag := sealed[len(sealed)-b.aead.Overhead():]
	body := sealed[:len(sealed)-b.aead.Overhead()]
	b64 := base64.RawURLEncoding
	return encPrefix + b64.EncodeToString(iv) + "." + b64.EncodeToString(tag) + "." + b64.EncodeToString(body), nil
}

func (b *SecretBox) Decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, encPrefix) {
		// 兼容未加密的明文（例如手工编辑数据文件后首次启动）。
		return value, nil
	}
	parts := strings.Split(strings.TrimPrefix(value, encPrefix), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("密文格式非法")
	}
	b64 := base64.RawURLEncoding
	iv, err1 := b64.DecodeString(parts[0])
	tag, err2 := b64.DecodeString(parts[1])
	body, err3 := b64.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return "", fmt.Errorf("密文格式非法")
	}
	if len(iv) != b.aead.NonceSize() {
		return "", fmt.Errorf("IV 长度非法")
	}
	sealed := append(append([]byte{}, body...), tag...)
	plain, err := b.aead.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("密钥不匹配或密文损坏")
	}
	return string(plain), nil
}

func RandomToken(nBytes int) (string, error) {
	raw := make([]byte, nBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// RandomPassword 生成无歧义字符的初始密码。
func RandomPassword(length int) (string, error) {
	const alphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz"
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
