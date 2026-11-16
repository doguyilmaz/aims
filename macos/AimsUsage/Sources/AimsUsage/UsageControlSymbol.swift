import SwiftUI

struct UsageControlSymbol: View {
    let icon: UsageControlIcon
    var pointSize: CGFloat = 13
    var fittedSize: CGFloat? = nil

    var body: some View {
        Group {
            if let fittedSize {
                Image(systemName: icon.symbol).resizable().scaledToFit()
                    .frame(width: fittedSize, height: fittedSize)
            } else {
                Image(systemName: icon.symbol)
            }
        }
        .symbolRenderingMode(.monochrome)
        .font(.system(size: pointSize, weight: .medium))
        .imageScale(.medium)
    }
}
