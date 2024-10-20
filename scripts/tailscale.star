load("//lib/alpine_kernel.star", "alpine_initramfs", "alpine_kernel", "alpine_kernel_fs", "alpine_modules_fs")

kernel_fs_320 = alpine_kernel_fs("3.20")

vm_params = {
    "initramfs": alpine_initramfs(kernel_fs_320),
    "kernel": alpine_kernel(kernel_fs_320),
    "cpu_cores": 1,
    "memory_mb": 1024,
    "storage_size": 1024,
}

vm_modfs = alpine_modules_fs(kernel_fs_320)

def make_vm(directives):
    return define.build_vm(
        directives = directives + [directive.run_command("interactive")],
        **vm_params
    )

plan = define.plan(
    builder = "alpine@3.20",
    packages = [
        query("tailscale"),
    ],
    tags = ["level3", "defaults"],
)

main = make_vm([
    plan,
    vm_modfs,
])