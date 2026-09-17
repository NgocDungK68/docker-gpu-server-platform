import { request } from "@/lib/api/api-client";

export type Role = "ADMIN" | "ORGANIZATION_USER";
export interface Organization { id: string; code: string; name: string; enabled: boolean; createdAt: string; updatedAt: string; }
export interface User { id: string; username: string; role: Role; organizationId: string; enabled: boolean; }
export interface Session { user: User; organization: Organization; }
export interface Enrollment { id: string; organizationId: string; serverId: string; displayName: string; labels: Record<string,string>; expiresAt: string; machineId?: string; revoked: boolean; }
export interface EnrollmentResult extends Enrollment { enrollmentToken: string; }
export interface UserInput { username: string; password: string; role: Role; organizationId: string; enabled: boolean; }

const post = <T>(path: string, body?: unknown) => request<T>(path, { method: "POST", ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
export const identityApi = {
  login: (username: string, password: string) => post<Session>("/auth/login", { username, password }),
  logout: () => post<{ loggedOut: boolean }>("/auth/logout"),
  me: () => request<Session>("/auth/me"),
  organizations: () => request<Organization[]>("/organizations"),
  saveOrganization: (input: Pick<Organization,"code"|"name"|"enabled">, id?: string) => post<Organization>(`/organizations${id ? `/${encodeURIComponent(id)}` : ""}`, input),
  users: () => request<User[]>("/users"),
  saveUser: (input: UserInput, id?: string) => post<User>(`/users${id ? `/${encodeURIComponent(id)}` : ""}`, input),
  enrollments: () => request<Enrollment[]>("/enrollments"),
  createEnrollment: (input: { displayName: string; labels: Record<string,string>; organizationId?: string }) => post<EnrollmentResult>("/enrollments",input),
  revokeEnrollment: (id: string) => post<{ revoked: boolean }>(`/enrollments/${encodeURIComponent(id)}/revoke`),
};
