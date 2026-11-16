import SwiftUI

struct SettingsNavigationLabel: View {
    let section: UsageSettingsSection

    var body: some View {
        Label {
            Text(section.title)
        } icon: {
            Image(systemName: section.symbol)
                .font(.system(size: 16, weight: .medium))
                .foregroundStyle(section.tint)
                .frame(width: 24, height: 24)
        }.padding(.vertical, 3)
    }
}
