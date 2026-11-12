import SwiftUI
import UsageCore

/// One account or model group, regardless of provider. All its windows stay visible.
struct UsageAccountTile: View {
    let provider: UsageProvider
    let row: UsageRow?

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 5) {
                    Image(systemName: ProviderStyle.symbol(provider.id))
                        .foregroundStyle(ProviderStyle.tint(provider.id))
                    Text(provider.id == "agy" ? "AGY" : ProviderStyle.title(provider))
                        .fontWeight(.semibold).fixedSize(horizontal: true, vertical: false)
                    Text("· \(row.map(ProviderStyle.accountName) ?? emptyLabel)")
                        .lineLimit(1).truncationMode(.middle)
                        .help("\(ProviderStyle.title(provider)) · \(row?.name ?? emptyLabel)")
                    Spacer(minLength: 0)
                    if let row {
                        UsageReadingDetails(provider: provider, row: row)
                    } else if let error = provider.error {
                        Image(systemName: "exclamationmark.circle").foregroundStyle(.orange).help(error)
                    }
                }
                .font(.system(size: 11))
                if let row, let windows = row.windows, !windows.isEmpty {
                    LazyVGrid(
                        columns: Array(repeating: GridItem(.flexible(), spacing: 8), count: min(2, windows.count)),
                        alignment: .leading, spacing: 8
                    ) {
                        ForEach(Array(windows.enumerated()), id: \.offset) { _, window in
                            UsageMetricView(
                                window: window, style: .gauges, tint: ProviderStyle.tint(provider.id),
                                stale: row.isStale(at: context.date) || row.error != nil || provider.error != nil,
                                now: context.date)
                        }
                    }
                } else {
                    Text(row == nil ? "Check data and CLI folders in Settings" : "Limits not reported")
                        .font(.system(size: 11)).foregroundStyle(.secondary)
                        .lineLimit(2)
                        .frame(height: 34, alignment: .leading)
                }
                HStack(spacing: 4) {
                    if let detail = row?.detail, !detail.isEmpty {
                        Text(detail).foregroundStyle(.secondary).help(detail)
                    } else if let plan = row?.plan, !plan.isEmpty {
                        Text(plan).foregroundStyle(.secondary)
                    }
                    Spacer(minLength: 0)
                    if let row, row.isStale(at: context.date) || row.error != nil || provider.error != nil {
                        Text("Cached").foregroundStyle(.secondary)
                    }
                    if let balance = row?.bankedResets {
                        Label("\(balance.available) banked", systemImage: "tray.full")
                            .foregroundStyle(ProviderStyle.tint(provider.id))
                    }
                }
                .font(.system(size: 10)).lineLimit(1).frame(height: 12)
            }
            .padding(8).frame(maxWidth: .infinity, alignment: .topLeading)
            .background(.background.opacity(0.55), in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(.primary.opacity(0.07)))
        }
    }

    private var emptyLabel: String {
        provider.id == "claude" || provider.id == "codex"
            ? "No accounts" : provider.installed ? "No reading" : "CLI unavailable"
    }
}
