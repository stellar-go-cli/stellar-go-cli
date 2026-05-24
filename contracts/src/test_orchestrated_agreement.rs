#[cfg(test)]
mod test {
    use crate::orchestrated_agreement::*;
    use soroban_sdk::{testutils::Address as _, Address, BytesN, Env, String, Symbol};

    fn setup() -> (Env, Address) {
        let env = Env::default();
        let owner = Address::generate(&env);
        env.mock_all_auths();
        
        // Initialize contract
        OrchestratedAgreementContract::__constructor(&env, owner.clone());
        
        (env, owner)
    }

    #[test]
    fn test_constructor_sets_owner() {
        let (env, owner) = setup();
        assert_eq!(OrchestratedAgreementContract::owner(&env), owner);
    }

    #[test]
    fn test_create_agreement() {
        let (env, owner) = setup();
        let initiator = Address::generate(&env);
        
        env.mock_all_auths();
        
        let id = OrchestratedAgreementContract::create_agreement(
            &env,
            initiator.clone(),
            None,
            None,
            86400, // 1 day dispute window
        );
        
        assert!(OrchestratedAgreementContract::agreement_exists(&env, id.clone()));
        assert_eq!(OrchestratedAgreementContract::get_agreement_count(&env), 1);
        
        let agreement = OrchestratedAgreementContract::get_agreement(&env, id);
        assert_eq!(agreement.state, AgreementState::Draft);
        assert_eq!(agreement.initiator, initiator);
    }

    #[test]
    fn test_attest_identity() {
        let (env, _owner) = setup();
        let initiator = Address::generate(&env);
        
        env.mock_all_auths();
        
        let id = OrchestratedAgreementContract::create_agreement(
            &env,
            initiator.clone(),
            None,
            None,
            86400,
        );
        
        let did = String::from_str(&env, "did:web:example.com");
        let attestation_hash = BytesN::from_array(&env, &[1u8; 32]);
        
        OrchestratedAgreementContract::attest_identity(
            &env,
            id.clone(),
            did,
            DIDMethod::Web,
            Symbol::new(&env, "national_id"),
            attestation_hash,
            None,
        );
        
        let agreement = OrchestratedAgreementContract::get_agreement(&env, id);
        assert_eq!(agreement.state, AgreementState::Active);
        assert!(agreement.identity.is_some());
    }
}
