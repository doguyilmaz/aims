import SwiftUI

struct TerminalCommandSheet: View {
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: 0) {
            Form { TerminalCommandSettings() }.formStyle(.grouped)
            HStack {
                Spacer()
                Button("Done") { dismiss() }.keyboardShortcut(.cancelAction)
            }.padding(16)
        }.frame(width: 440, height: 300)
    }
}
