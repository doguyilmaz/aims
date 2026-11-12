import SwiftUI
import UsageCore

struct CompactProviderCard: View {
    let provider: UsageProvider

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 6) {
                    Image(systemName: ProviderStyle.symbol(provider.id))
                        .foregroundStyle(ProviderStyle.tint(provider.id))
                    Text(ProviderStyle.title(provider)).fontWeight(.semibold)
                    Spacer()
                    if let error = provider.error, !error.isEmpty {
                        Image(systemName: "exclamationmark.triangle").foregroundStyle(.orange).help(error)
                    }
                }.font(.system(size: 12))
                if provider.rows.isEmpty {
                    Text(
                        provider.id == "claude" || provider.id == "codex"
                            ? "No accounts · choose data in Settings, or run aims init."
                            : provider.installed ? "No reading yet" : "CLI unavailable"
                    )
                    .font(.system(size: 11)).foregroundStyle(.secondary)
                }
                ForEach(provider.rows) { row in
                    VStack(alignment: .leading, spacing: 4) {
                        HStack(alignment: .center, spacing: 12) {
                            VStack(alignment: .leading, spacing: 3) {
                                HStack(spacing: 2) {
                                    Text(ProviderStyle.accountName(row)).fontWeight(.medium)
                                        .lineLimit(2).truncationMode(.middle).help(row.name)
                                    UsageReadingDetails(provider: provider, row: row)
                                }.font(.system(size: 11))
                                if row.isStale(at: context.date) || row.error != nil || provider.error != nil {
                                    Label("Cached", systemImage: "clock").foregroundStyle(.secondary)
                                } else if let plan = row.plan, row.bankedResets == nil {
                                    Text(plan).foregroundStyle(.secondary).lineLimit(1)
                                }
                                if let balance = row.bankedResets {
                                    Text("\(balance.available) banked").foregroundStyle(ProviderStyle.tint(provider.id))
                                }
                            }
                            .font(.system(size: 10)).frame(width: 84, alignment: .leading)
                            if let windows = row.windows, !windows.isEmpty {
                                LazyVGrid(
                                    columns: Array(
                                        repeating: GridItem(.flexible(), spacing: 14), count: min(2, windows.count)),
                                    alignment: .leading, spacing: 10
                                ) {
                                    ForEach(Array(windows.enumerated()), id: \.offset) { _, window in
                                        UsageMetricView(
                                            window: window, style: .compact, tint: ProviderStyle.tint(provider.id),
                                            stale: row.isStale(at: context.date) || row.error != nil
                                                || provider.error != nil,
                                            now: context.date)
                                    }
                                }.frame(maxWidth: .infinity)
                            } else {
                                Text("Limits not reported").font(.system(size: 11)).foregroundStyle(.secondary)
                            }
                        }
                        if let detail = row.detail, !detail.isEmpty {
                            Text(detail).font(.system(size: 11)).foregroundStyle(.secondary).lineLimit(1).help(detail)
                        }
                    }
                }
            }
            .padding(10).frame(maxWidth: .infinity, alignment: .leading)
            .background(.background.opacity(0.55), in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(.primary.opacity(0.07)))
        }
    }
}
