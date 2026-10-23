package application

import (
	"context"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func scopedJobs(ctx context.Context,jobs []domain.Job) []domain.Job {
	result:=[]domain.Job{}
	for _,job:=range jobs { if domain.CanAccess(ctx,job.OrganizationID) { result=append(result,job) } }
	return result
}

func (c *ControlPlane) enabledOrganizations(ctx context.Context) (map[string]bool,error) {
	result:=map[string]bool{}
	if c.metadata==nil { return result,nil }
	items,err:=c.metadata.ListOrganizations(ctx)
	if err!=nil { return nil,err }
	for _,o:=range items { result[o.ID]=o.Enabled }
	return result,nil
}
