import SwiftUI
import UsageCore

struct ProviderCard: View {
    let provider: UsageProvider
    let surface: UsageSurface
    @State private var expandedLimits = false
    @State private var expandedResets = false
    @Environment(\.colorScheme) private var colorScheme

    private var tint: Color { ProviderStyle.tint(provider.id) }
    private var hasDetails: Bool { provider.rows.contains { ($0.windows?.count ?? 0) > 1 } }
    private var isModelGroup: Bool { provider.id == "agy" }

    var body: some View {
        VStack(alignment: .leading, spacing: surface == .panel ? 14 : 10) {
            header
            if provider.rows.isEmpty {
                emptyReading
            } else if isModelGroup || (surface == .panel && provider.rows.count > 1) {
                LazyVGrid(
                    columns: Array(
                        repeating: GridItem(.flexible(), spacing: 12, alignment: .top),
                        count: isModelGroup && surface == .popover ? 4 : 2), spacing: 16
                ) {
                    ForEach(provider.rows) { row in
                        UsageAccountView(
                            row: row, tint: tint, surface: surface, tile: true,
                            showDetails: expandedLimits || surface == .panel)
                    }
                }
            } else {
                VStack(spacing: 10) {
                    ForEach(provider.rows) { row in
                        UsageAccountView(
                            row: row, tint: tint, surface: surface, tile: false,
                            showDetails: expandedLimits || surface == .panel)
                    }
                }
            }
            if let error = provider.error, !error.isEmpty {
                Label("Refresh failed · cached reading", systemImage: "exclamationmark.triangle")
                    .font(.system(size: 10)).foregroundStyle(.orange).help(error)
            }
            if expandedResets {
                Divider()
                ForEach(provider.rows) { row in
                    VStack(alignment: .leading, spacing: 6) {
                        Text(ProviderStyle.accountName(row)).font(.system(size: 11, weight: .semibold))
                        ResetInventoryView(balance: row.bankedResets)
                    }
                }
            }
        }
        .padding(surface == .panel ? 16 : 12)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .background {
            RoundedRectangle(cornerRadius: 18, style: .continuous)
                .fill(Color(nsColor: .controlBackgroundColor))
                .overlay {
                    RoundedRectangle(cornerRadius: 18, style: .continuous)
                        .fill(
                            LinearGradient(
                                colors: [tint.opacity(colorScheme == .dark ? 0.10 : 0.055), .clear],
                                startPoint: .topLeading, endPoint: .bottomTrailing))
                }
        }
        .overlay(RoundedRectangle(cornerRadius: 18).strokeBorder(.primary.opacity(0.055), lineWidth: 1))
        .shadow(color: .black.opacity(colorScheme == .dark ? 0.12 : 0.035), radius: 8, y: 3)
    }

    private var header: some View {
        HStack(spacing: 6) {
            Image(systemName: ProviderStyle.symbol(provider.id))
                .font(.system(size: 12, weight: .semibold)).foregroundStyle(tint)
                .frame(width: 21, height: 23)
            Text(ProviderStyle.title(provider)).font(.system(size: 13, weight: .semibold)).lineLimit(1)
            Spacer(minLength: 2)
            if provider.id == "codex" || provider.rows.contains(where: { $0.bankedResets != nil }) {
                Button {
                    expandedResets.toggle()
                } label: {
                    Image(systemName: "tray.full.fill")
                }
                .font(.system(size: 11, weight: .medium)).buttonStyle(.plain)
                .foregroundStyle(tint).padding(5).background(tint.opacity(0.10), in: Circle())
                .accessibilityLabel("Banked reset details for \(ProviderStyle.title(provider))")
                .help("Banked reset inventory, separately for each account")
            }
            if hasDetails && surface == .popover {
                Button {
                    expandedLimits.toggle()
                } label: {
                    Image(systemName: expandedLimits ? "chevron.up" : "chevron.down")
                        .font(.system(size: 9, weight: .semibold)).frame(width: 14, height: 22)
                }.buttonStyle(.plain).foregroundStyle(.secondary)
                    .accessibilityLabel("Toggle all limits for \(ProviderStyle.title(provider))")
                    .help("Show all limit windows")
            }
        }
    }

    private var emptyReading: some View {
        VStack(alignment: .leading, spacing: 7) {
            UsageGauge(window: nil, tint: tint, stale: false, size: 52)
            Text(
                provider.id == "claude" || provider.id == "codex"
                    ? "No accounts configured" : provider.installed ? "No reading yet" : "CLI unavailable"
            )
            .font(.system(size: 11)).foregroundStyle(.secondary)
            if provider.id == "claude" || provider.id == "codex" {
                Text("Choose your aims data folder in Settings, or run aims init.")
                    .font(.system(size: 11)).foregroundStyle(.secondary)
            }
        }
        .help(
            provider.installed
                ? "Refresh to check this provider. Claude uses its native cached status line."
                : "Install the provider CLI or choose its folder in Settings.")
    }
}
