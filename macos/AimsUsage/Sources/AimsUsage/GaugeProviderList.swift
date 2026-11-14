import SwiftUI
import UsageCore

struct GaugeProviderList: View {
    let snapshot: UsageSnapshot
    let refreshFailed: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(snapshot.displayProviders) { provider in
                VStack(alignment: .leading, spacing: 2) {
                    HStack(spacing: 6) {
                        ProviderMark(id: provider.id, size: 15)
                        Text(ProviderStyle.title(provider))
                            .font(.system(size: 12, weight: .semibold)).lineLimit(1)
                        if let error = provider.error {
                            Image(systemName: "exclamationmark.circle")
                                .foregroundStyle(.orange).help(error)
                        }
                        Rectangle().fill(.primary.opacity(0.08)).frame(height: 1)
                    }.frame(height: 18)
                    if provider.rows.isEmpty {
                        Text(emptyLabel(provider)).font(.system(size: 11)).foregroundStyle(.secondary)
                            .frame(maxWidth: .infinity, minHeight: 24, alignment: .leading)
                    } else {
                        ForEach(provider.rows) { row in
                            GaugeAccountRow(provider: provider, row: row, refreshFailed: refreshFailed)
                        }
                    }
                }
            }
        }.frame(maxWidth: .infinity, alignment: .leading)
    }

    private func emptyLabel(_ provider: UsageProvider) -> String {
        if provider.id == "claude" || provider.id == "codex" { return "No accounts" }
        return provider.installed ? "No reading" : "CLI unavailable"
    }
}
