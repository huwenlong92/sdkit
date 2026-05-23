package request

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
)

type MultipartPart struct {
	FieldName   string
	FileName    string
	ContentType string
	Reader      io.Reader
	Headers     textproto.MIMEHeader
}

func FieldPart(name, value string) MultipartPart {
	return MultipartPart{
		FieldName: name,
		Reader:    bytes.NewBufferString(value),
	}
}

func FilePart(fieldName, fileName, contentType string, r io.Reader) MultipartPart {
	return MultipartPart{
		FieldName:   fieldName,
		FileName:    fileName,
		ContentType: contentType,
		Reader:      r,
	}
}

func encodeMultipart(parts []MultipartPart) ([]byte, string, error) {
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	for _, part := range parts {
		if part.Reader == nil {
			_ = writer.Close()
			return nil, "", ErrNilMultipartPart
		}
		header := make(textproto.MIMEHeader)
		for key, values := range part.Headers {
			header[key] = append([]string(nil), values...)
		}
		if header.Get("Content-Disposition") == "" {
			disposition := fmt.Sprintf(`form-data; name=%q`, part.FieldName)
			if part.FileName != "" {
				disposition += fmt.Sprintf(`; filename=%q`, part.FileName)
			}
			header.Set("Content-Disposition", disposition)
		}
		if part.ContentType != "" && header.Get("Content-Type") == "" {
			header.Set("Content-Type", part.ContentType)
		}
		if part.FileName != "" && header.Get("Content-Type") == "" {
			header.Set("Content-Type", http.DetectContentType(nil))
		}
		w, err := writer.CreatePart(header)
		if err != nil {
			_ = writer.Close()
			return nil, "", err
		}
		if _, err := io.Copy(w, part.Reader); err != nil {
			_ = writer.Close()
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}
