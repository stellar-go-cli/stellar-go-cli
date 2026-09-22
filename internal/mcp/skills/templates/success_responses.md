# Success Response Templates

## Overview
Standardized response templates for successful operations, providing clear confirmation, relevant details, and helpful next steps for users of the Stellar Go CLI platform.

## Transaction Success Templates

### Payment Completed
**Trigger**: Successful payment execution

**Response Template**:
```markdown
✅ **Payment Successful!**

**Transaction Details:**
• Amount: [amount] [asset_code]
• To: [recipient_address]
• From: [sender_address]
• Transaction Hash: [tx_hash]
• Network: [network]
• Rail: [payment_rail]

**Confirmation Status:**
⏳ Pending → ✅ Confirmed (after ~5 seconds)

**View on Explorer:**
[explorer_url]

**What's Next:**
• Check your updated balance
• Save the transaction hash for records
• Set up payment notifications
• Create recurring payment if this is regular

**Need Help?**
• "show payment history"
• "check my balance"
• "set up payment alerts"
```

### Swap Completed
**Trigger**: Successful swap execution

**Response Template**:
```markdown
✅ **Swap Completed Successfully!**

**Swap Details:**
• From: [amount_from] [asset_from]
• To: [amount_to] [asset_to]
• Exchange Rate: [exchange_rate]
• Price Impact: [price_impact]
• Fees: [total_fees] [fee_asset]
• Network: [network]

**Transaction Hash:** [tx_hash]
**Execution Time:** [execution_time]

**Performance:**
• Rate vs Market: [market_comparison]
• Slippage: [slippage_percentage]
• Liquidity Used: [liquidity_info]

**What's Next:**
• Check your new [asset_to] balance
• Set up price alerts for [asset_from]/[asset_to]
• Review swap history
• Consider setting up automated swaps

**Related Actions:**
• "show my balance"
• "monitor [asset_from]/[asset_to] pair"
• "set price alert for [asset_to]"
```

### Asset Created
**Trigger**: Successful token/asset creation

**Response Template**:
```markdown
✅ **Asset Created Successfully!**

**Asset Details:**
• Name: [asset_name]
• Symbol: [asset_symbol]
• Type: [asset_type] (FT/NFA)
• Total Supply: [total_supply]
• Decimals: [decimal_places]
• Network: [network]

**Contract Information:**
• Asset ID: [asset_id]
• Contract Address: [contract_address]
• Creator: [creator_address]
• Created At: [creation_timestamp]

**Special Features:**
• Carbon Credits: [carbon_status]
• Metadata: [metadata_status]
• Verification: [verification_status]

**What's Next:**
• Create a trustline for your new asset
• Set up initial liquidity
• Create distribution plan
• Register on exchanges

**Immediate Actions:**
• "trust [asset_symbol] from my wallet"
• "show asset details for [asset_symbol]"
• "create liquidity pool for [asset_symbol]"
```

### Wallet Connected
**Trigger**: Successful wallet connection/import

**Response Template**:
```markdown
✅ **Wallet Connected Successfully!**

**Wallet Details:**
• Address: [wallet_address]
• Type: [wallet_type]
• Network: [network]
• Provider: [provider_type]
• Status: [connection_status]

**Security Setup:**
• Passkey: [passkey_status]
• Backup: [backup_status]
• Multi-sig: [multisig_status]

**Current Balance:**
• XLM: [xlm_balance]
• Other Assets: [other_assets_count]

**First Steps:**
• Fund your wallet (if balance is low)
• Set up security features
• Explore available assets
• Make your first transaction

**Quick Actions:**
• "fund my wallet from testnet faucet"
• "show my balance"
• "list available assets"
• "set up wallet security"
```

---

## Information Display Templates

### Balance Information
**Trigger**: Balance check request

**Response Template**:
```markdown
💰 **Current Balance Information**

**Active Wallet:** [wallet_address]
**Network:** [network]

**Asset Balances:**
• XLM: [xlm_balance] (Native)
• USDC: [usdc_balance] (Stablecoin)
• EURC: [eurc_balance] (Stablecoin)
• [custom_asset_1]: [balance_1]
• [custom_asset_2]: [balance_2]

**Total Value (USD):** [total_usd_value]

**Account Status:**
• Minimum Reserve: [reserve_requirement] XLM
• Available Balance: [available_balance] XLM
• Trustlines: [trustlines_count] active

**Recent Activity:**
• Last Transaction: [last_transaction_time]
• Last 24h Volume: [daily_volume]

**Quick Actions:**
• "fund my wallet" (if low balance)
• "show transaction history"
• "check asset prices"
• "set up balance alerts"
```

### System Status
**Trigger**: System status check

**Response Template**:
```markdown
📊 **Stellar Go CLI System Status**

**Overall Health:** ✅ Healthy

**Network Status:**
• Stellar Network: ✅ Operational
• Horizon API: ✅ Connected
• Response Time: [response_time]ms
• Last Block: [last_block_height]

**Services Status:**
• Wallet Service: ✅ Active
• Swap Engine: ✅ Operational
• Payment Rails: ✅ Available
• Asset Registry: ✅ Updated

**Performance Metrics:**
• Uptime: [uptime_percentage]%
• API Success Rate: [success_rate]%
• Average Latency: [avg_latency]ms

**Active Wallets:** [active_wallets_count]
**Current Network:** [current_network]
**Version:** [system_version]

**Recent Updates:**
• [update_1]
• [update_2]

**Need Help?**
• "check network connectivity"
• "test service status"
• "view system logs"
```

### Asset Information
**Trigger**: Asset details request

**Response Template**:
```markdown
📋 **Asset Information: [asset_code]**

**Basic Details:**
• Name: [asset_name]
• Symbol: [asset_code]
• Type: [asset_type]
• Issuer: [issuer_address]
• Network: [network]

**Market Data:**
• Price (USD): [price_usd]
• 24h Change: [price_change_24h]
• 24h Volume: [volume_24h]
• Market Cap: [market_cap]
• Total Supply: [total_supply]

**Technical Details:**
• Decimals: [decimal_places]
• Trustline Required: [trustline_required]
• Minimum Balance: [min_balance]
• Fee Rate: [fee_rate]

**Quality Score:** [quality_score]/100
**Risk Level:** [risk_level]

**Trustline Status:** [trustline_status]
**Your Balance:** [user_balance]

**Actions Available:**
• "trust [asset_code] from [issuer_address]"
• "buy [asset_code] with XLM"
• "sell [asset_code] for XLM"
• "set price alert for [asset_code]"
```

---

## Configuration Success Templates

### Network Switched
**Trigger**: Successful network change

**Response Template**:
```markdown
✅ **Network Switched Successfully!**

**New Network Configuration:**
• Network: [new_network]
• Horizon URL: [horizon_url]
• Network Passphrase: [network_passphrase]
• Switch Time: [switch_timestamp]

**What Changed:**
• Active wallet: [wallet_status]
• Asset availability: [asset_changes]
• Fee structure: [fee_changes]
• Market conditions: [market_changes]

**Important Notes:**
⚠️ [network_specific_warning]
📝 [configuration_note]

**Verification:**
• Testnet: Free XLM available from faucet
• Mainnet: Real market conditions apply

**Next Steps:**
• Check your wallet on new network
• Verify asset availability
• Test a small transaction
• Update any saved addresses

**Quick Actions:**
• "check my balance on [new_network]"
• "list available assets"
• "fund my wallet" (if on testnet)
```

### Security Configured
**Trigger**: Security setup completion

**Response Template**:
```markdown
🔒 **Security Configuration Complete!**

**Security Features Enabled:**
• Passkey Authentication: ✅ [passkey_status]
• Multi-signature: ✅ [multisig_status]
• Backup Phrase: ✅ [backup_status]
• Access Control: ✅ [access_control_status]

**Security Level:** [security_level]

**Protection Against:**
• Unauthorized access: ✅ Protected
• Key theft: ✅ Mitigated
• Account takeover: ✅ Prevented
• Transaction fraud: ✅ Secured

**Recovery Options:**
• Passkey recovery: [passkey_recovery]
• Backup phrase: [backup_phrase_available]
• Multi-sig recovery: [multisig_recovery]

**Security Recommendations:**
• Store backup phrase securely
• Enable transaction notifications
• Regular security audits
• Keep software updated

**Next Steps:**
• Test security features
• Set up transaction alerts
• Create emergency recovery plan
• Review security best practices

**Quick Actions:**
• "test passkey authentication"
• "set up transaction alerts"
• "view security settings"
```

---

## Monitoring and Alert Templates

### Alert Triggered
**Trigger**: Price alert, balance threshold, or system alert

**Response Template**:
```markdown
🚨 **Alert Triggered!**

**Alert Type:** [alert_type]
**Severity:** [severity_level]

**Alert Details:**
• Trigger: [trigger_condition]
• Current Value: [current_value]
• Threshold: [threshold_value]
• Timestamp: [alert_timestamp]

**Context:**
[additional_context]

**Recommended Actions:**
• [action_1]
• [action_2]
• [action_3]

**Quick Response:**
• "[quick_action_command]"
• "[alternative_action_command]"

**Alert Management:**
• "modify this alert"
• "pause alerts temporarily"
• "delete this alert"
• "create similar alert"
```

### Monitoring Started
**Trigger**: Background monitoring initiated

**Response Template**:
```markdown
📊 **Monitoring Started Successfully!**

**Monitoring Configuration:**
• Target: [monitoring_target]
• Parameters: [monitoring_params]
• Duration: [monitoring_duration]
• Update Frequency: [update_frequency]

**What I'm Watching:**
• [metric_1]: [current_value_1]
• [metric_2]: [current_value_2]
• [metric_3]: [current_value_3]

**Alert Conditions:**
• [condition_1]: [threshold_1]
• [condition_2]: [threshold_2]
• [condition_3]: [threshold_3]

**Notifications:**
• Email: [email_status]
• Push: [push_status]
• In-app: [in_app_status]

**Control Commands:**
• "stop monitoring"
• "pause monitoring"
• "check monitoring status"
• "modify monitoring settings"

**Expected Duration:** [expected_end_time]
**Progress Updates:** [update_frequency]
```

---

## Workflow Completion Templates

### Multi-Step Process Complete
**Trigger**: Complex workflow finished

**Response Template**:
```markdown
🎉 **Workflow Completed Successfully!**

**Process Summary:**
• Workflow: [workflow_name]
• Duration: [total_duration]
• Steps Completed: [steps_completed]/[total_steps]
• Success Rate: [success_percentage]%

**Completed Steps:**
✅ [step_1]: [step_1_result]
✅ [step_2]: [step_2_result]
✅ [step_3]: [step_3_result]

**Final Results:**
• [result_1]: [value_1]
• [result_2]: [value_2]
• [result_3]: [value_3]

**Resources Used:**
• Time: [time_used]
• Fees: [total_fees]
• Network Calls: [network_calls]

**What's Changed:**
• [change_1]
• [change_2]
• [change_3]

**Next Recommendations:**
• [recommendation_1]
• [recommendation_2]
• [recommendation_3]

**Learn More:**
• "explain what we just did"
• "show detailed results"
• "set up similar workflow"
```

### Batch Operations Complete
**Trigger**: Multiple operations finished

**Response Template**:
```markdown
✅ **Batch Operations Complete!**

**Batch Summary:**
• Total Operations: [total_operations]
• Successful: [successful_count]
• Failed: [failed_count]
• Success Rate: [success_percentage]%

**Operation Breakdown:**
• [operation_type_1]: [count_1] successful, [failed_1] failed
• [operation_type_2]: [count_2] successful, [failed_2] failed
• [operation_type_3]: [count_3] successful, [failed_3] failed

**Total Impact:**
• Amount Processed: [total_amount]
• Fees Paid: [total_fees]
• Time Saved: [time_saved]

**Failed Operations:**
• [failure_1]: [failure_reason_1]
• [failure_2]: [failure_reason_2]

**Retry Options:**
• "retry failed operations"
• "retry operation [operation_id]"
• "investigate failures"

**Next Steps:**
• Review successful operations
• Handle any failures
• Update records
• Set up automation

**Quick Actions:**
• "show operation details"
• "export results"
• "create similar batch"
```

---

## Educational Success Templates

### Learning Objective Met
**Trigger**: User successfully learns concept

**Response Template**:
```markdown
🎓 **Great Job! You've Mastered [concept_name]!**

**Demonstrated Skills:**
• ✅ [skill_1]
• ✅ [skill_2]
• ✅ [skill_3]

**Your Progress:**
• Current Level: [current_level]
• Next Milestone: [next_milestone]
• Overall Progress: [progress_percentage]%

**What You Can Now Do:**
• [capability_1]
• [capability_2]
• [capability_3]

**Practice Suggestions:**
• Try "[practice_exercise_1]"
• Attempt "[practice_exercise_2]"
• Explore "[advanced_topic]"

**Next Learning:**
• [next_concept_1]
• [next_concept_2]
• [next_concept_3]

**Keep Going!**
You're building strong skills with Stellar Go CLI. Each concept mastered makes you more capable!

**Want to continue learning or practice what you've learned?**
```

---

## Personalization Templates

### Preference Adaptation
**Trigger**: System learns user preference

**Response Template**:
```markdown
🧠 **I've Learned Your Preferences!**

**Observed Patterns:**
• You prefer [preference_1]: [pattern_1]
• You usually [preference_2]: [pattern_2]
• You like [preference_3]: [pattern_3]

**Adaptations Made:**
• [adaptation_1]: Now default for you
• [adaptation_2]: Automatically configured
• [adaptation_3]: Personalized responses

**Better Experience:**
• Faster responses to your style
• More relevant suggestions
• Predictive assistance
• Reduced friction

**Examples of New Behavior:**
• Instead of [old_behavior], I now [new_behavior]
• I automatically [automated_action] when [condition]
• I suggest [personalized_suggestion] based on your patterns

**Feedback:**
Am I getting better at anticipating your needs?

**Adjustments:**
• "keep these preferences"
• "modify preference [preference]"
• "reset to defaults"
• "show all my preferences"
```

---

## Template Usage Guidelines

### Dynamic Variables
Replace `[variable_name]` with actual values:
- `[amount]`: Transaction amount
- `[asset_code]`: Asset symbol
- `[wallet_address]`: Stellar address
- `[tx_hash]`: Transaction hash
- `[network]`: Network name
- `[timestamp]`: Time information

### Context Adaptation
Modify templates based on:
- User experience level
- Transaction complexity
- Security implications
- Educational context

### Tone Adjustment
Adjust tone based on:
- Success importance
- User expertise
- Operation complexity
- Relationship history

### Multi-language Support
Templates should support:
- Multiple languages
- Regional formatting
- Cultural considerations
- Local terminology

These templates provide consistent, helpful, and contextually appropriate responses for successful operations across the Stellar Go CLI platform.
