package helm

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"

	rspb "helm.sh/helm/v3/pkg/release"
)

var b64 = base64.StdEncoding

var magicGzip = []byte{0x1f, 0x8b, 0x08}

// decodeRelease decodes Helm 3 release Secret data (base64+gzip+json).
// Algorithm matches helm.sh/helm/v3/pkg/storage/driver.
func decodeRelease(data string) (*rspb.Release, error) {
	b, err := b64.DecodeString(data)
	if err != nil {
		return nil, err
	}
	if len(b) > 3 && bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b2, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}
	var rls rspb.Release
	if err := json.Unmarshal(b, &rls); err != nil {
		return nil, err
	}
	return &rls, nil
}

// encodeRelease encodes a release for test fixtures (Helm 3 Secret format).
func encodeRelease(rls *rspb.Release) (string, error) {
	b, err := json.Marshal(rls)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err = w.Write(b); err != nil {
		return "", err
	}
	w.Close()
	return b64.EncodeToString(buf.Bytes()), nil
}
