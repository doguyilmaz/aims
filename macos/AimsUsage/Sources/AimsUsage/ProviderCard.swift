import SwiftUI
import UsageCore

struct ProviderCard: View {
    let provider: UsageProvider

    private var tint: Color {
        switch provider.id {
        case "claude": .orange
        case "codex": .cyan
        case "kiro": .purple
        case "grok": .pink
        case "agy": .mint
        default: .indigo
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(spacing: 8) {
                Text(String(provider.name.prefix(1))).font(.system(size: 12, weight: .bold, design: .rounded))
                    .frame(width: 24, height: 24).background(tint.opacity(0.17), in: RoundedRectangle(cornerRadius: 7))
                    .foregroundStyle(tint)
                Text(provider.name).font(.system(size: 13, weight: .semibold))
                Spacer()
                if !provider.installed {
                    Text("NOT INSTALLED").font(.system(size: 8, weight: .medium)).foregroundStyle(.tertiary)
                }
            }
            if let error = provider.error, !error.isEmpty {
                Label(error, systemImage: "exclamationmark.circle").font(.caption2).foregroundStyle(.orange)
            }
            if provider.rows.isEmpty {
                Text(
                    provider.installed
                        ? (provider.id == "claude" || provider.id == "codex"
                            ? "No configured accounts. Choose your aims data folder in Settings, or run aims init."
                            : "No reading yet. Refresh to check this provider.")
                        : "CLI not found. Install it or add its folder in Settings."
                )
                .font(.caption).foregroundStyle(.secondary)
            }
            ForEach(provider.rows) { row in
                UsageAccountView(row: row, showBanked: provider.id == "codex" || provider.id == "claude", tint: tint)
                if row.id != provider.rows.last?.id { Divider().opacity(0.4) }
            }
            if provider.id == "agy" {
                Text("Each reported model group keeps its own limits. Unreported groups are unavailable.")
                    .font(.system(size: 9)).foregroundStyle(.tertiary)
            }
        }
        .padding(14)
        .background(.white.opacity(0.045), in: RoundedRectangle(cornerRadius: 16))
        .overlay(RoundedRectangle(cornerRadius: 16).strokeBorder(.white.opacity(0.075), lineWidth: 1))
    }
}
