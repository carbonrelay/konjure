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
	"os"

	"github.com/spf13/cobra"
	"github.com/thestormforge/konjure/pkg/konjure"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
)

func NewRootCommand(version, refspec, date string) *cobra.Command {
	r := konjure.Resources{}
	f := &konjure.Filter{}
	w := &konjure.Writer{}

	cmd := &cobra.Command{
		Use:              "konjure INPUT...",
		Short:            "Manifest, appear!",
		Version:          version,
		SilenceUsage:     true,
		TraverseChildren: true,
		Annotations: map[string]string{
			"BuildRefspec": refspec,
			"BuildDate":    date,
		},
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			w.Writer = cmd.OutOrStdout()
			f.DefaultReader = cmd.InOrStdin()

			if len(args) > 0 {
				r = append(r, konjure.NewResource(args...))
			} else {
				r = append(r, konjure.NewResource("-"))
			}

			f.WorkingDirectory, err = os.Getwd()

			if !w.KeepReaderAnnotations {
				w.ClearAnnotations = append(w.ClearAnnotations,
					kioutil.PathAnnotation,
					filters.FmtAnnotation,
				)
			}

			return
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return kio.Pipeline{
				Inputs:  []kio.Reader{r},
				Filters: []kio.Filter{f},
				Outputs: []kio.Writer{w},
			}.Execute()
		},
	}

	cmd.Flags().IntVarP(&f.Depth, "depth", "d", 100, "limit the number of times expansion can happen")
	cmd.Flags().StringVarP(&f.LabelSelector, "selector", "l", "", "label query to filter on")
	cmd.Flags().StringVar(&f.Kind, "kind", "", "keep only resource matching the specified kind")
	cmd.Flags().BoolVar(&f.KeepStatus, "keep-status", false, "retain status fields, if present")
	cmd.Flags().BoolVar(&f.KeepComments, "keep-comments", true, "retain YAML comments")
	cmd.Flags().BoolVar(&f.Format, "format", false, "format output to Kubernetes conventions")
	cmd.Flags().BoolVarP(&f.RecursiveDirectories, "recurse", "r", false, "recursively process directories")
	cmd.Flags().StringVarP(&w.Format, "output", "o", "yaml", "set the output format")
	cmd.Flags().BoolVar(&w.KeepReaderAnnotations, "keep-annotations", false, "retain annotations used for processing")
	cmd.Flags().BoolVar(&w.Sort, "sort", false, "sort output prior to writing")

	cmd.AddCommand(
		NewHelmCommand(),
		NewJsonnetCommand(),
		NewSecretCommand(),
	)

	return cmd
}
