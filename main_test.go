package main

import (
	"log"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk"
	"github.com/awslabs/goformation/v5"
	"github.com/awslabs/goformation/v5/cloudformation"
	"github.com/stretchr/testify/assert"
)

func testProvisioning(t *testing.T, template *cloudformation.Template) {
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

func testSecurityGroups(t *testing.T, template *cloudformation.Template) {
	sg, _ := template.GetEC2SecurityGroupWithName("StrapiServiceSecurityGroup6B02743C")
	// allows outbound
	assert.Equal(t, "0.0.0.0/0", sg.SecurityGroupEgress[0].CidrIp, "SecurityGroup Egress expected 0.0.0.0/0")

	// allows inbound
	sg, _ = template.GetEC2SecurityGroupWithName("StrapiServiceLBSecurityGroupB6F6DC82")
	assert.Equal(t, 80, sg.SecurityGroupIngress[0].FromPort)
	assert.Equal(t, 80, sg.SecurityGroupIngress[0].ToPort)
	assert.Equal(t, "0.0.0.0/0", sg.SecurityGroupIngress[0].CidrIp)
}

func testTask(t *testing.T, template *cloudformation.Template) {
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

func TestStrapiStack(t *testing.T) {
	app := awscdk.NewApp(nil)

	stack := NewStrapiStack(app, "TestStack", nil)

	// load cfn template
	artifact := app.Synth(nil).GetStackArtifact(stack.ArtifactId())
	templatePath := artifact.TemplateFullPath()
	template, err := goformation.Open(*templatePath)
	if err != nil {
		log.Fatalf("There was an error processing the template: %s", err)
	}

	templateTests := []struct {
		name string
		test func(t *testing.T, template *cloudformation.Template)
	}{
		{"Service provisioning", testProvisioning},
		{"Security groups", testSecurityGroups},
		{"Task definition", testTask},
	}

	for _, tc := range templateTests {
		t.Run(tc.name, func(t *testing.T) {
			tc.test(t, template)
		})
	}

}
