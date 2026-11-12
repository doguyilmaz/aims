import SwiftUI
import UsageCore

struct UsageReadingDetails: View {
    enum LabelStyle { case info, banked, compactBanked }
    let provider: UsageProvider
    let row: UsageRow
    var labelStyle: LabelStyle = .info
    @State private var presented = false

    var body: some View {
        Button {
            presented.toggle()
        } label: {
            if labelStyle != .info, let balance = row.bankedResets {
                Label(
                    labelStyle == .banked ? "\(balance.available) banked" : "\(balance.available)",
                    systemImage: "tray.full"
                )
                .font(.system(size: 11)).foregroundStyle(ProviderStyle.tint(provider.id)).fixedSize()
            } else {
                Image(systemName: row.error != nil || provider.error != nil ? "exclamationmark.circle" : "info.circle")
                    .foregroundStyle(row.error != nil || provider.error != nil ? .orange : .secondary)
                    .frame(width: 18, height: 18).contentShape(Rectangle())
            }
        }
        .buttonStyle(.plain)
        .help("Reading details and banked resets for \(row.name)")
        .accessibilityLabel(
            "\(labelStyle == .info ? "Details" : "Banked resets") for \(ProviderStyle.title(provider)) \(row.name)"
        )
        .popover(isPresented: $presented) {
            ScrollView {
                VStack(alignment: .leading, spacing: 10) {
                    Text("\(ProviderStyle.title(provider)) · \(ProviderStyle.accountName(row))")
                        .font(.system(size: 13, weight: .semibold))
                    if let plan = row.plan { Text(plan).foregroundStyle(.secondary) }
                    if let detail = row.detail { Text(detail).textSelection(.enabled) }
                    Text(
                        row.observedAt.map { "Reported \($0.formatted(date: .abbreviated, time: .shortened))" }
                            ?? "Observation time not reported"
                    )
                    .foregroundStyle(.secondary)
                    if let error = row.error ?? provider.error {
                        Text(error).foregroundStyle(.orange).textSelection(.enabled)
                    }
                    if provider.id == "codex" || row.bankedResets != nil {
                        Divider()
                        ResetInventoryView(balance: row.bankedResets)
                    }
                }
                .font(.system(size: 12)).padding(16).frame(maxWidth: .infinity, alignment: .leading)
            }.frame(width: 290).frame(maxHeight: 360)
        }
    }
}
