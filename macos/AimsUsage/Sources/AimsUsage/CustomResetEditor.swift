import SwiftUI

struct CustomResetEditor: View {
    @Bindable var model: UsageModel
    @Environment(\.dismiss) private var dismiss
    @State private var provider = "codex"
    @State private var row = ""
    @State private var label = "Custom reset"
    @State private var date = Date().addingTimeInterval(3_600)
    @State private var failed = false

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Custom reset reminder").font(.headline)
            Text("A personal reminder. It never changes the provider’s quota or redeems a banked reset.")
                .font(.caption).foregroundStyle(.secondary)
            Form {
                Picker("Provider", selection: $provider) {
                    ForEach((model.snapshot?.providers ?? []).filter { !$0.rows.isEmpty }) { p in Text(p.name).tag(p.id)
                    }
                }
                Picker("Account / group", selection: $row) {
                    ForEach(model.snapshot?.providers.first { $0.id == provider }?.rows ?? []) { r in
                        Text(r.name).tag(r.id)
                    }
                }
                TextField("Label", text: $label)
                DatePicker("Remind me", selection: $date, in: Date()..., displayedComponents: [.date, .hourAndMinute])
            }
            if failed { Text(model.error ?? "Could not save the reminder.").font(.caption).foregroundStyle(.orange) }
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Add reminder") {
                    Task {
                        if await model.addReminder(provider: provider, row: row, label: label, at: date) {
                            dismiss()
                        } else {
                            failed = true
                        }
                    }
                }.buttonStyle(.borderedProminent).disabled(
                    row.isEmpty || label.trimmingCharacters(in: .whitespaces).isEmpty || label.utf8.count > 120
                        || model.isRefreshing)
            }
        }
        .padding(24).frame(width: 420)
        .scrollIndicators(.automatic).background(UsageScrollStyle())
        .environment(\.usageSlimScrollbars, model.slimScrollbars)
        .onAppear {
            if model.snapshot?.providers.first(where: { $0.id == provider })?.rows.isEmpty != false {
                provider = model.snapshot?.providers.first(where: { !$0.rows.isEmpty })?.id ?? "codex"
            }
            selectRow()
        }
        .onChange(of: provider) { _, _ in selectRow() }
    }

    private func selectRow() {
        row = model.snapshot?.providers.first { $0.id == provider }?.rows.first?.id ?? ""
    }
}
