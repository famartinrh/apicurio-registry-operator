package apicurioregistry

import (
	ar "github.com/Apicurio/apicurio-registry-operator/pkg/apis/apicur/v1alpha1"
	ocp_apps "github.com/openshift/api/apps/v1"
	"os"
)

var _ ControlFunction = &ImageOcpCF{}

// This CF takes care of keeping the "image" section of the CRD applied.
type ImageOcpCF struct {
	ctx              *Context
	deploymentEntry  ResourceCacheEntry
	deploymentExists bool
	existingImage    string
	targetImage      string
}

func NewImageOcpCF(ctx *Context) ControlFunction {
	return &ImageOcpCF{
		ctx:              ctx,
		deploymentEntry:  nil,
		deploymentExists: false,
		existingImage:    RC_EMPTY_NAME,
		targetImage:      RC_EMPTY_NAME,
	}
}

func (this *ImageOcpCF) Describe() string {
	return "ImageOcpCF"
}

func (this *ImageOcpCF) Sense() {
	// Observation #1
	// Get the cached Deployment (if it exists and/or the value)
	deploymentEntry, deploymentExists := this.ctx.GetResourceCache().Get(RC_KEY_DEPLOYMENT_OCP)
	this.deploymentEntry = deploymentEntry
	this.deploymentExists = deploymentExists

	// Observation #2
	// Get the existing image name (if present)
	this.existingImage = RC_EMPTY_NAME
	if this.deploymentExists {
		for i, c := range deploymentEntry.GetValue().(*ocp_apps.DeploymentConfig).Spec.Template.Spec.Containers {
			if c.Name == this.ctx.GetConfiguration().GetAppName() {
				this.existingImage = deploymentEntry.GetValue().(*ocp_apps.DeploymentConfig).Spec.Template.Spec.Containers[i].Image
			}
		} // TODO report a problem if not found?
	}

	// Observation #3
	// Get the target image name
	persistence := ""
	if specEntry, exists := this.ctx.GetResourceCache().Get(RC_KEY_SPEC); exists {
		spec := specEntry.GetValue().(*ar.ApicurioRegistry).Spec
		this.targetImage = spec.Image.Name
		persistence = spec.Configuration.Persistence
	}

	if this.targetImage == "" {
		envImage := ""
		switch persistence {
		case "", "mem":
			envImage = os.Getenv(ENV_OPERATOR_REGISTRY_IMAGE_MEM)
		case "kafka":
			envImage = os.Getenv(ENV_OPERATOR_REGISTRY_IMAGE_KAFKA)
		case "streams":
			envImage = os.Getenv(ENV_OPERATOR_REGISTRY_IMAGE_STREAMS)
		case "jpa":
			envImage = os.Getenv(ENV_OPERATOR_REGISTRY_IMAGE_JPA)
		case "infinispan":
			envImage = os.Getenv(ENV_OPERATOR_REGISTRY_IMAGE_INFINISPAN)
		}
		if envImage != "" {
			this.targetImage = envImage
		} else {
			this.ctx.GetLog().WithValues("type", "Warning").
				Info("WARNING: The operand image is not selected. " +
					"Set the 'spec.configuration.persistence' property in your 'apicurioregistry' resource " +
					"to select the appropriate Service Registry image. You can override using 'spec.image.name'.")
		}
	}

	// Update state
	this.ctx.GetConfiguration().SetConfig(CFG_STA_IMAGE, this.existingImage)
}

func (this *ImageOcpCF) Compare() bool {
	// Condition #1
	// Deployment exists
	// Condition #2
	// Existing image is not the same as the target image (assuming it is never empty)
	return this.deploymentEntry != nil &&
		this.existingImage != this.targetImage
}

func (this *ImageOcpCF) Respond() {
	// Response #1
	// Patch the resource
	this.deploymentEntry.ApplyPatch(func(value interface{}) interface{} {
		deployment := value.(*ocp_apps.DeploymentConfig).DeepCopy()
		for i, c := range deployment.Spec.Template.Spec.Containers {
			if c.Name == this.ctx.GetConfiguration().GetAppName() {
				deployment.Spec.Template.Spec.Containers[i].Image = this.targetImage
			}
		} // TODO report a problem if not found?
		return deployment
	})
}
