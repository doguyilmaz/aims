import SwiftUI

struct ProviderMark: View {
    let id: String
    var size: CGFloat = 18

    var body: some View {
        Group {
            if let image = NSImage(named: "aims.provider.\(id)") {
                Image(nsImage: image).resizable().renderingMode(.template).scaledToFit()
            } else {
                Image(systemName: ProviderStyle.symbol(id)).resizable().scaledToFit()
            }
        }
        .foregroundStyle(ProviderStyle.tint(id)).frame(width: size, height: size)
        .accessibilityHidden(true)
    }
}
