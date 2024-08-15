package check

import (
	"crypto/md5"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"

	"lukechampine.com/blake3"
)

const BLOCKSIZE = 2 << 10 << 7 // kb

func Hashfile(path string, hashAlgo string, perfMonBytes func(int64)) (string, error) {
	var h hash.Hash
	switch hashAlgo {
	case "md5":
		h = md5.New()
	case "sha512":
		h = sha512.New()
	case "blake3":
		h = blake3.New(32, nil)
	default:
		return "", errors.New("algo '" + hashAlgo + "' is unknown.")
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buf := make([]byte, BLOCKSIZE)
	for {
		bytesRead, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		if bytesRead == 0 {
			break
		}
		h.Write(buf[:bytesRead])
		if perfMonBytes != nil {
			perfMonBytes(int64(bytesRead))
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func HashMd5(data []byte) string {
	h := md5.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
