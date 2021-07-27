package main

import (
	"log"
	"os"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk"
	"github.com/awslabs/goformation/v5"
	"github.com/awslabs/goformation/v5/cloudformation"
	"github.com/stretchr/testify/assert"
)

var (
	template *cloudformation.Template
)

func TestProvisioning(t *testing.T) {
	service, err := template.GetECSServiceWithName("StrapiServiceC5D46785")
	if err != nil {
		log.Fatalf("Could not get Fargate service: %s", err)
	}
	assert.Equal(t, "FARGATE", service.LaunchType, "LaunchType expected to be FARGATE")
	assert.Equal(t, 1, service.DesiredCount, "DesiredCount expected to be 1")
	assert.Equal(t, 60, service.HealthCheckGracePeriodSeconds)

	// accepts :80
	assert.Equal(t, 80, service.LoadBalancers[0].ContainerPort, "LoadBalancer expected to be on port 80")
}

func TestSecurityGroups(t *testing.T) {
	sg, _ := template.GetEC2SecurityGroupWithName("StrapiServiceSecurityGroup6B02743C")
	// allows outbound
	assert.Equal(t, "0.0.0.0/0", sg.SecurityGroupEgress[0].CidrIp, "SecurityGroup Egress expected 0.0.0.0/0")

	// allows inbound
	sg, _ = template.GetEC2SecurityGroupWithName("StrapiServiceLBSecurityGroupB6F6DC82")
	assert.Equal(t, 80, sg.SecurityGroupIngress[0].FromPort)
	assert.Equal(t, 80, sg.SecurityGroupIngress[0].ToPort)
	assert.Equal(t, "0.0.0.0/0", sg.SecurityGroupIngress[0].CidrIp)
}

func TestTask(t *testing.T) {
	task, _ := template.GetECSTaskDefinitionWithName("StrapiDefinition74B6E401")
	container := task.ContainerDefinitions[0]

	// includes mount point
	assert.Equal(t, "/srv/app", container.MountPoints[0].ContainerPath)

	// maps to port 80
	assert.Equal(t, 80, container.PortMappings[0].ContainerPort)

	// contains creds
	for _, secret := range container.Secrets {
		assert.Contains(t, []string{"DATABASE_PASSWORD", "DATABASE_USERNAME"}, secret.Name)
	}

	// volume attached
	target, _ := template.GetEFSMountTargetWithName("StrapiFileSystemEfsMountTarget1683E5808")
	assert.Equal(t, target.FileSystemId, task.Volumes[0].EFSVolumeConfiguration.FilesystemId)
}

func TestMain(m *testing.M) {
	app := awscdk.NewApp(nil)
	stack := NewStrapiStack(app, "TestStack", nil)

	// load cfn template
	artifact := app.Synth(nil).GetStackArtifact(stack.ArtifactId())
	templatePath := artifact.TemplateFullPath()
	template, _ = goformation.Open(*templatePath)

	os.Exit(m.Run())
}
