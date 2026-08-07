ALTER TABLE approvals DROP CONSTRAINT approvals_status_check;
ALTER TABLE approvals ADD CONSTRAINT approvals_status_check CHECK (status IN ('PENDING','APPROVED','READY_FOR_EXECUTION_CONFIRMATION','REJECTED','EXPIRED','CONSUMED'));

ALTER TABLE approvals DROP CONSTRAINT approvals_check;
ALTER TABLE approvals ADD CONSTRAINT approvals_check CHECK (
  (status = 'PENDING' AND decision = 'PENDING' AND decided_at IS NULL) OR
  (status IN ('APPROVED','READY_FOR_EXECUTION_CONFIRMATION') AND decision = 'APPROVED' AND decided_at IS NOT NULL) OR
  (status = 'REJECTED' AND decision = 'REJECTED' AND decided_at IS NOT NULL) OR
  (status = 'EXPIRED' AND ((decision = 'PENDING' AND decided_at IS NULL) OR (decision = 'APPROVED' AND decided_at IS NOT NULL))) OR
  (status = 'CONSUMED' AND decision = 'APPROVED' AND decided_at IS NOT NULL)
);
