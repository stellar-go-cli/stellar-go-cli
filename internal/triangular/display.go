package triangular

import "fmt"

// DisplayResults formats triangular results for display
func DisplayResults(results []TriangularResult) string {
	output := "\n  Triangular Arbitrage Results\n"
	output += "  ─────────────────────────────\n\n"

	for i, r := range results {
		output += fmt.Sprintf("  [%d] %s\n", i+1, r.Path.Name)
		output += fmt.Sprintf("      Combined Rate: %.6f (%.4f%% deviation)\n",
			r.Path.CombinedRate, r.Path.Deviation*100)
		output += fmt.Sprintf("      Start: %.2f XLM → End: %.6f XLM\n",
			r.StartingXLM, r.FinalXLM)
		output += fmt.Sprintf("      Net: %.6f XLM (%.4f%%)\n",
			r.NetProfitXLM, r.ProfitPercent)

		if r.Path.IsOpportunity {
			output += "      ⚠️  OPPORTUNITY DETECTED\n"
		}

		// Show statistical analysis if available
		if r.ZScore != 0 || r.Volatility > 0 {
			output += fmt.Sprintf("      Z-Score: %.4f", r.ZScore)
			if r.IsMeanReversion {
				output += " ⚠️ MEAN REVERSION"
			}
			output += "\n"
		}
		if r.Volatility > 0 {
			output += fmt.Sprintf("      Volatility: %.4f%% | Score: %d/100\n",
				r.Volatility*100, r.OpportunityScore)
		}

		output += "\n"
	}

	return output
}
