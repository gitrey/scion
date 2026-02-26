/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * Shared types for server and client
 */

/**
 * User role enumeration
 */
export type UserRole = 'admin' | 'member' | 'viewer';

/**
 * User information
 */
export interface User {
  id: string;
  email: string;
  name: string;
  avatar?: string | undefined;
  role?: UserRole | undefined;
}

/**
 * Admin user information from the Hub API (GET /api/v1/users)
 */
export interface AdminUser {
  id: string;
  email: string;
  displayName: string;
  avatarUrl?: string;
  role: UserRole;
  status: 'active' | 'suspended';
  created: string;
  lastLogin?: string;
}

/**
 * Group type enumeration
 */
export type GroupType = 'explicit' | 'grove_agents';

/**
 * Group information from the Hub API (GET /api/v1/groups)
 */
export interface AdminGroup {
  id: string;
  name: string;
  slug: string;
  description?: string;
  groupType: GroupType;
  groveId?: string;
  parentId?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  ownerId?: string;
  createdBy?: string;
  created: string;
  updated: string;
}

/**
 * Group member information
 */
export interface GroupMember {
  groupId: string;
  memberType: 'user' | 'group' | 'agent';
  memberId: string;
  role: 'member' | 'admin' | 'owner';
  addedAt: string;
  addedBy?: string;
}

/**
 * Initial page data passed from SSR to client
 */
export interface PageData {
  /** Current URL path */
  path: string;
  /** Page title */
  title: string;
  /** Current user (if authenticated) */
  user?: User | undefined;
  /** Additional page-specific data */
  data?: Record<string, unknown> | undefined;
}

/**
 * Route definition for client-side routing
 */
export interface RouteConfig {
  path: string;
  component: string;
  action?: () => Promise<void>;
}

/**
 * Grove status enumeration
 */
export type GroveStatus = 'active' | 'inactive' | 'error';

/**
 * Grove information from the Hub API
 */
export interface Grove {
  id: string;
  name: string;
  slug?: string;
  path: string;
  gitRemote?: string;
  status: GroveStatus;
  visibility?: string;
  labels?: Record<string, string>;
  agentCount: number;
  createdAt: string;
  updatedAt: string;
}

/**
 * Agent status enumeration
 */
export type AgentStatus =
  | 'running'
  | 'stopped'
  | 'provisioning'
  | 'cloning'
  | 'error'
  | 'idle'
  | 'busy'
  | 'waiting_for_input'
  | 'completed';

/**
 * Agent information from the Hub API
 */
export interface Agent {
  id: string;
  name: string;
  groveId: string;
  grove?: string;
  template: string;
  status: AgentStatus;
  taskSummary?: string;
  message?: string;
  lastSeen?: string;
  createdAt: string;
  updatedAt: string;
}

/**
 * Template information from the Hub API
 */
export interface Template {
  id: string;
  name: string;
  slug: string;
  displayName?: string;
  description?: string;
  harness: string;
  status: string;
  scope: string;
  createdAt: string;
  updatedAt: string;
}

/**
 * Runtime Broker status enumeration
 */
export type BrokerStatus = 'online' | 'offline' | 'degraded';

/**
 * Capabilities advertised by a Runtime Broker
 */
export interface BrokerCapabilities {
  webPTY: boolean;
  sync: boolean;
  attach: boolean;
}

/**
 * Runtime profile available on a broker
 */
export interface BrokerProfile {
  name: string;
  type: string;
  available: boolean;
}

/**
 * Runtime Broker information from the Hub API
 */
export interface RuntimeBroker {
  id: string;
  name: string;
  slug: string;
  version: string;
  status: BrokerStatus;
  connectionState: string;
  lastHeartbeat: string;
  capabilities?: BrokerCapabilities;
  profiles?: BrokerProfile[];
  autoProvide: boolean;
  endpoint?: string;
  createdAt: string;
  updatedAt: string;
}
