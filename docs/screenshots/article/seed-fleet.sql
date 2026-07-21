-- Article screenshot seed: realistic multi-cluster drift data for Omastx demos.

INSERT INTO clusters (id, name, api_server_url, context, kubeconfig_enc, kubeconfig_nonce, rbac_report, schedule_cron, status, last_scan_at)
VALUES
  ('11111111-1111-1111-1111-111111111111', 'prod-eu', 'https://prod-eu.example.com:6443', 'prod-eu',
   decode('00','hex'), decode('00','hex'),
   '{"images_ok":true,"helm_ok":true,"checked_at":"2026-07-13T09:00:00Z","permissions":[]}'::jsonb,
   -- Yearly cron avoids scheduler catch-up scans against dummy kubeconfigs.
   '0 0 1 1 *', 'connected', now() - interval '20 minutes'),
  ('22222222-2222-2222-2222-222222222222', 'prod-us', 'https://prod-us.example.com:6443', 'prod-us',
   decode('00','hex'), decode('00','hex'),
   '{"images_ok":true,"helm_ok":false,"checked_at":"2026-07-13T09:00:00Z","permissions":[]}'::jsonb,
   '0 0 1 1 *', 'degraded', now() - interval '2 hours'),
  ('33333333-3333-3333-3333-333333333333', 'staging', 'https://staging.example.com:6443', 'staging',
   decode('00','hex'), decode('00','hex'),
   '{"images_ok":true,"helm_ok":true,"checked_at":"2026-07-13T09:00:00Z","permissions":[]}'::jsonb,
   '0 0 1 1 *', 'error', now() - interval '1 hour');

INSERT INTO scans (id, cluster_id, started_at, finished_at, status, stats)
VALUES ('a1111111-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111',
        now() - interval '21 minutes', now() - interval '20 minutes', 'done',
        '{"total":9,"current":4,"patch":2,"minor":1,"major":1,"deprecated":1,"unknown":0,"errors":0,"images":7,"helm":2,"auth_required":0}'::jsonb);

INSERT INTO artifacts (id, cluster_id, kind, namespace, owner_kind, owner_name, identity, installed_version, source_meta)
VALUES
  ('b0000001-0000-0000-0000-000000000001','11111111-1111-1111-1111-111111111111','image','checkout','Deployment','checkout-api','docker.io/acme/checkout-api','2.4.1','{"registry":"docker.io"}'),
  ('b0000001-0000-0000-0000-000000000002','11111111-1111-1111-1111-111111111111','image','checkout','Deployment','checkout-worker','docker.io/acme/checkout-worker','1.9.0','{"registry":"docker.io"}'),
  ('b0000001-0000-0000-0000-000000000003','11111111-1111-1111-1111-111111111111','image','payments','Deployment','payments-api','ghcr.io/acme/payments-api','3.1.0','{"registry":"ghcr.io"}'),
  ('b0000001-0000-0000-0000-000000000004','11111111-1111-1111-1111-111111111111','image','payments','StatefulSet','ledger-db','docker.io/library/postgres','16.2','{"registry":"docker.io"}'),
  ('b0000001-0000-0000-0000-000000000005','11111111-1111-1111-1111-111111111111','image','platform','Deployment','gateway','docker.io/library/nginx','1.25.3','{"registry":"docker.io"}'),
  ('b0000001-0000-0000-0000-000000000006','11111111-1111-1111-1111-111111111111','image','platform','DaemonSet','log-agent','docker.io/acme/log-agent','0.8.2','{"registry":"docker.io"}'),
  ('b0000001-0000-0000-0000-000000000007','11111111-1111-1111-1111-111111111111','image','platform','Deployment','legacy-cache','docker.io/library/redis','5.0.14','{"registry":"docker.io"}'),
  ('b0000001-0000-0000-0000-000000000008','11111111-1111-1111-1111-111111111111','helm','platform','HelmRelease','ingress-nginx','ingress-nginx','4.8.0','{"chart_repo":"https://kubernetes.github.io/ingress-nginx","release":"ingress-nginx"}'),
  ('b0000001-0000-0000-0000-000000000009','11111111-1111-1111-1111-111111111111','helm','payments','HelmRelease','cert-manager','cert-manager','1.13.0','{"chart_repo":"https://charts.jetstack.io","release":"cert-manager"}');

INSERT INTO observations (scan_id, artifact_id, installed_version, latest_version, drift_class, drift_score, releases_behind, confidence)
VALUES
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000001','2.4.1','2.4.1','current',0,0,NULL),
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000002','1.9.0','1.9.3','patch',3,2,NULL),
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000003','3.1.0','3.1.0','current',0,0,NULL),
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000004','16.2','16.4','patch',2,1,NULL),
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000005','1.25.3','1.27.0','minor',200,4,NULL),
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000006','0.8.2','0.8.2','current',0,0,NULL),
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000007','5.0.14','5.0.14','deprecated',0,0,NULL),
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000008','4.8.0','4.10.0','major',20000,2,0.85),
  ('a1111111-0000-0000-0000-000000000001','b0000001-0000-0000-0000-000000000009','1.13.0','1.13.0','current',0,0,1.0);

INSERT INTO scans (id, cluster_id, started_at, finished_at, status, stats)
VALUES ('a2222222-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222',
        now() - interval '2 hours 1 minute', now() - interval '2 hours', 'done',
        '{"total":3,"current":2,"patch":0,"minor":0,"major":0,"deprecated":0,"unknown":1,"errors":0,"images":3,"helm":0,"auth_required":0}'::jsonb);

INSERT INTO artifacts (id, cluster_id, kind, namespace, owner_kind, owner_name, identity, installed_version, source_meta)
VALUES
  ('b0000002-0000-0000-0000-000000000001','22222222-2222-2222-2222-222222222222','image','checkout','Deployment','checkout-api','docker.io/acme/checkout-api','2.4.1','{"registry":"docker.io"}'),
  ('b0000002-0000-0000-0000-000000000002','22222222-2222-2222-2222-222222222222','image','platform','Deployment','gateway','docker.io/library/nginx','1.27.0','{"registry":"docker.io"}'),
  ('b0000002-0000-0000-0000-000000000003','22222222-2222-2222-2222-222222222222','image','platform','Deployment','internal-tool','registry.internal.example.com/acme/internal-tool','sha-9f21ab3','{"registry":"registry.internal.example.com","resolve_status":"auth_required"}');

INSERT INTO observations (scan_id, artifact_id, installed_version, latest_version, drift_class, drift_score)
VALUES
  ('a2222222-0000-0000-0000-000000000001','b0000002-0000-0000-0000-000000000001','2.4.1','2.4.1','current',0),
  ('a2222222-0000-0000-0000-000000000001','b0000002-0000-0000-0000-000000000002','1.27.0','1.27.0','current',0),
  ('a2222222-0000-0000-0000-000000000001','b0000002-0000-0000-0000-000000000003','sha-9f21ab3',NULL,'unknown',0);

INSERT INTO scans (id, cluster_id, started_at, finished_at, status, error)
VALUES ('a3333333-0000-0000-0000-000000000001', '33333333-3333-3333-3333-333333333333',
        now() - interval '65 minutes', now() - interval '63 minutes', 'error',
        'registry unreachable: dial tcp: i/o timeout');
