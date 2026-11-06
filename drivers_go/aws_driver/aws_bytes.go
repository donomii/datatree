package awsdriver

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"datahumper/drivers_go/driverlib"
)

// Only the provider sees descriptors. Every consuming filter gets file bytes.
func readAWSChild(key string, value []byte, offset int64, length int64) ([]byte, error) {
	switch {
	case !strings.HasPrefix(key, "s3-bytes:"):
		return driverlib.SliceRange(value, offset, length), nil
	default:
	}
	state, err := parseMountState(value)
	switch {
	case err != nil:
		return nil, err
	case state.View != viewS3Object:
		return nil, fmt.Errorf("invalid S3 byte child: expected object descriptor, got %q", state.View)
	default:
	}
	config, err := resolveAWSConfig()
	switch {
	case err != nil:
		return nil, err
	default:
	}
	// The CLI metadata limit is not a file-size limit. Local and remote files
	// use the same whole-document contract for filters such as ZIP and JSON.
	// For finite ranges retain only requested bytes while draining the stream.
	args := s3ReadArgs(state)
	assertReadOnlyAWSInvocation(args)
	output := s3RangeBuffer{skip: offset, remaining: length}
	stderr := awsBoundedBuffer{limit: config.maxOutputBytes}
	cmd := exec.Command(config.executable, args...)
	cmd.Stdout, cmd.Stderr = &output, &stderr
	err = cmd.Run()
	switch {
	case err != nil || stderr.exceeded():
		return nil, fmt.Errorf("cannot read S3 object %q from bucket %q: expected complete file bytes, got %v with %d stderr bytes (limit %d): %s", state.Key, state.Identifier, err, stderr.total, config.maxOutputBytes, stderr.data.String())
	default:
		return output.data.Bytes(), nil
	}
}

type s3RangeBuffer struct {
	skip, remaining int64
	data            bytes.Buffer
}

func (output *s3RangeBuffer) Write(data []byte) (int, error) {
	total := len(data)
	switch {
	case output.skip < 0:
		return total, nil
	case output.skip >= int64(len(data)):
		output.skip -= int64(len(data))
		return total, nil
	default:
		data = data[output.skip:]
		output.skip = 0
	}
	switch {
	case output.remaining >= 0 && int64(len(data)) > output.remaining:
		data = data[:output.remaining]
	default:
	}
	_, err := output.data.Write(data)
	switch {
	case output.remaining >= 0:
		output.remaining -= int64(len(data))
	default:
	}
	return total, err
}
