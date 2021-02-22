/*
Copyright 2021 GramLabs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package command

import (
	"github.com/spf13/cobra"
	"github.com/thestormforge/konjure/internal/readers"
	"github.com/thestormforge/konjure/pkg/konjure"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

func NewRootCommand() *cobra.Command {
	r := &readers.ResourceReader{}
	f := &konjure.Filter{}
	w := &konjure.Writer{}

	// TODO We should have another filter that only keeps resources matching a labelSelector, annotationSelector, group, kind, version, etc.
	// TODO What about other "utility" filters/output settings like striping comments?

	cmd := &cobra.Command{
		Use:              "konjure INPUT...",
		Short:            "Manifest, appear!",
		SilenceUsage:     true,
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			r.Resources = args
			r.Reader = cmd.InOrStdin()
			w.Writer = cmd.OutOrStdout()

			p := kio.Pipeline{
				Inputs:  []kio.Reader{r},
				Filters: []kio.Filter{f},
				Outputs: []kio.Writer{w},
			}

			return p.Execute()
		},
	}

	cmd.Flags().IntVarP(&f.Depth, "depth", "d", 100, "limit the number of times expansion can happen")
	cmd.Flags().StringVarP(&w.Format, "output", "o", "yaml", "set the output format")
	cmd.Flags().BoolVar(&w.Sort, "sort", false, "sort output prior to writing")

	cmd.AddCommand(
		NewHelmCommand(),
		NewJsonnetCommand(),
		NewSecretCommand(),
	)

	return cmd
}