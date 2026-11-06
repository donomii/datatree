package kubernetesdriver

import (
	"fmt"
	"os"
	"strings"
)

func logKubernetesStartFailure(err error) {
	fmt.Fprintf(os.Stderr, "Kubernetes driver cannot start: failed to fetch the mount state: %v\n", err)
}

func logKubernetesResourceEncodeFailure(state MountState, err error) {
	fmt.Fprintf(os.Stderr, "Failed to encode kubernetes resources for context %q namespace %q type %q: %v\n", state.Context, state.Namespace, state.Type, err)
}

func logKubernetesManifestFailure(message string) {
	fmt.Fprintln(os.Stderr, message)
}

func logPartialKubectlFailure(args []string, err error, stderr string) {
	fmt.Fprintf(os.Stderr, "kubectl %s reported a partial failure, continuing with its partial output: %v: %s\n", strings.Join(args, " "), err, strings.TrimSpace(stderr))
}
