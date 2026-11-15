import SwiftUI

struct UsageControlSymbol: View {
    let icon: UsageControlIcon
    var pointSize: CGFloat = 13

    var body: some View {
        Image(systemName: icon.symbol)
            .symbolRenderingMode(.monochrome)
            .font(.system(size: pointSize, weight: .medium))
            .imageScale(.medium)
    }
}
