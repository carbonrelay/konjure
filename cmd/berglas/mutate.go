package berglas

import (
	"fmt"
	"path"

	"github.com/GoogleCloudPlatform/berglas/pkg/berglas"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/kustomize/v3/pkg/ifc"
	"sigs.k8s.io/kustomize/v3/pkg/resmap"
	"sigs.k8s.io/kustomize/v3/pkg/types"
)

type BerglasMutator struct {
	resMapFactory *resmap.Factory
	loader        ifc.Loader
	genOpts       *types.GeneratorOptions
	secrets       resmap.ResMap
}

func NewBerglasMutator(f *resmap.Factory, l ifc.Loader, o *types.GeneratorOptions) *BerglasMutator {
	m := &BerglasMutator{
		resMapFactory: f,
		loader:        l,
		genOpts:       o,
	}

	if o != nil {
		m.secrets = resmap.New()
	}

	return m
}

func (m *BerglasMutator) FlushSecrets(rm resmap.ResMap) error {
	var err error
	if m.secrets != nil {
		err = rm.AppendAll(m.secrets)
		m.secrets.Clear()
	}
	return err
}

func (m *BerglasMutator) Mutate(template *corev1.PodTemplateSpec) (bool, error) {
	if m.genOpts != nil {
		return m.mutateTemplateWithSecrets(template)
	}
	return m.mutateTemplate(template), nil
}

// Mutation with secrets does the secret lookup now instead of in the container

func (m *BerglasMutator) mutateTemplateWithSecrets(template *corev1.PodTemplateSpec) (bool, error) {
	mutated := false

	for i, c := range template.Spec.InitContainers {
		if c, didMutate, err := m.mutateContainerWithSecrets(&c); err != nil {
			return mutated, err
		} else if didMutate {
			mutated = true
			template.Spec.InitContainers[i] = *c
		}
	}

	for i, c := range template.Spec.Containers {
		if c, didMutate, err := m.mutateContainerWithSecrets(&c); err != nil {
			return mutated, err
		} else if didMutate {
			mutated = true
			template.Spec.Containers[i] = *c
		}
	}

	for _, r := range m.secrets.Resources() {
		mutated = true
		template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
			Name: r.GetName(),
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.GetName(),
				},
			},
		})
	}

	return mutated, nil
}

func (m *BerglasMutator) mutateContainerWithSecrets(c *corev1.Container) (*corev1.Container, bool, error) {
	mutated := false
	for _, e := range c.Env {
		if berglas.IsReference(e.Value) {
			// Parse the environment variable value as Berglas reference
			r, err := berglas.ParseReference(e.Value)
			if err != nil {
				return c, mutated, err
			}

			// Do not allow environment variables to contain sensitive information in the generated manifests
			if r.Filepath() == "" {
				// TODO Should this be an error?
				continue
			}

			// Create a resource map with a secret that we can merge into the existing collection
			args := types.SecretArgs{}
			args.Name = r.Bucket()
			args.FileSources = []string{fmt.Sprintf("%s=%s/%s", r.Filepath(), r.Bucket(), r.Object())}
			sm, err := m.resMapFactory.FromSecretArgs(m.loader, m.genOpts, args)
			if err != nil {
				return c, mutated, err
			}

			// Merge the generated secret into the existing collection
			err = m.secrets.AbsorbAll(sm)
			if err != nil {
				return c, mutated, err
			}

			// Replace the environment variable value with the path
			mutated = true
			e.Value = r.Filepath()

			// Add a mount to get the secret where it was requested
			// TODO There is going to be a problem with "tempfile" since the OS the build runs on may have a different TMP_DIR convention
			c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
				Name:      r.Bucket(),
				MountPath: e.Value,
				SubPath:   path.Base(e.Value),
				ReadOnly:  true,
			})
		}
	}

	return c, mutated, nil
}

// The rest of this is the mutating webhook from https://github.com/GoogleCloudPlatform/berglas/tree/master/examples/kubernetes

const (
	berglasContainer   = "gcr.io/berglas/berglas:latest"
	binVolumeName      = "berglas-bin"
	binVolumeMountPath = "/berglas/bin/"
)

var binInitContainer = corev1.Container{
	Name:            "copy-berglas-bin",
	Image:           berglasContainer,
	ImagePullPolicy: corev1.PullIfNotPresent,
	Command:         []string{"sh", "-c", fmt.Sprintf("cp /bin/berglas %s", binVolumeMountPath)},
	VolumeMounts: []corev1.VolumeMount{
		{
			Name:      binVolumeName,
			MountPath: binVolumeMountPath,
		},
	},
}

var binVolume = corev1.Volume{
	Name: binVolumeName,
	VolumeSource: corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{
			Medium: corev1.StorageMediumMemory,
		},
	},
}

var binVolumeMount = corev1.VolumeMount{
	Name:      binVolumeName,
	MountPath: binVolumeMountPath,
	ReadOnly:  true,
}

func (m *BerglasMutator) mutateTemplate(template *corev1.PodTemplateSpec) bool {
	mutated := false

	for i, c := range template.Spec.InitContainers {
		c, didMutate := m.mutateContainer(&c)
		if didMutate {
			mutated = true
			template.Spec.InitContainers[i] = *c
		}
	}

	for i, c := range template.Spec.Containers {
		c, didMutate := m.mutateContainer(&c)
		if didMutate {
			mutated = true
			template.Spec.Containers[i] = *c
		}
	}

	if mutated {
		template.Spec.Volumes = append(template.Spec.Volumes, binVolume)
		template.Spec.InitContainers = append([]corev1.Container{binInitContainer}, template.Spec.InitContainers...)
	}

	return mutated
}

func (m *BerglasMutator) mutateContainer(c *corev1.Container) (*corev1.Container, bool) {
	if !m.hasBerglasReferences(c.Env) {
		return c, false
	}
	if len(c.Command) == 0 {
		// TODO Should this be an error?
		return c, false
	}
	c.VolumeMounts = append(c.VolumeMounts, binVolumeMount)
	original := append(c.Command, c.Args...)
	c.Command = []string{binVolumeMountPath + "berglas"}
	c.Args = append([]string{"exec", "--local", "--"}, original...)
	return c, true
}

func (m *BerglasMutator) hasBerglasReferences(env []corev1.EnvVar) bool {
	for _, e := range env {
		if berglas.IsReference(e.Value) {
			return true
		}
	}
	return false
}
