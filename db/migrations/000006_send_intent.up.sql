ALTER TABLE intents DROP CONSTRAINT intents_intent_type_check;
ALTER TABLE intents ADD CONSTRAINT intents_intent_type_check
    CHECK (intent_type IN ('SEND','PAYROLL','SWAP','BRIDGE','ANS_REGISTRATION'));
