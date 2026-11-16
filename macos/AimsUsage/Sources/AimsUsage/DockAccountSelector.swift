import SwiftUI
import UsageCore

struct DockAccountSelector: View {
    @Bindable var model: UsageModel
    let provider: UsageProvider
    var label = "Account"
    @Environment(\.controlSize) private var controlSize
    private var isMini: Bool { controlSize == .mini }

    var body: some View {
        let selected = model.dockAccount(for: provider.id)
        HStack(spacing: isMini ? 6 : 8) {
            Text(label).font(.system(size: isMini ? 10 : 11)).foregroundStyle(.secondary)
            ScrollView(.horizontal) {
                HStack(spacing: 4) {
                    ForEach(DockAccountFocus.options(for: provider, including: selected)) { account in
                        Button {
                            model.setDockAccount(account, for: provider.id)
                        } label: {
                            Text(account.title(in: provider)).font(
                                .system(size: isMini ? 10 : 11, weight: selected == account ? .semibold : .regular)
                            )
                            .lineLimit(1).fixedSize().padding(.horizontal, isMini ? 7 : 8).padding(
                                .vertical, isMini ? 3 : 4
                            )
                            .background(
                                selected == account
                                    ? ProviderStyle.tint(provider.id).opacity(0.2) : Color.primary.opacity(0.05),
                                in: Capsule()
                            )
                            .overlay(
                                Capsule().stroke(Color.primary.opacity(selected == account ? 0.2 : 0.07), lineWidth: 1))
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel(
                            "\(ProviderStyle.title(provider)) Dock account, \(account.title(in: provider))"
                        )
                        .accessibilityValue(selected == account ? "Selected" : "")
                        .help(help(for: account))
                    }
                }
            }.scrollIndicators(.hidden).frame(height: isMini ? 22 : 26)
        }
    }

    private func help(for account: DockAccountFocus) -> String {
        switch account {
        case .all: "Combine account readings in Edge Dock. Display preference only."
        case .account(let id): "Show \(id) usage in Edge Dock. This is a display preference."
        }
    }
}
