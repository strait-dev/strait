const presets = [
	{ name: "micro", vcpu: "1", ram: "256 MB", perSec: "$0.000017", perHour: "$0.061" },
	{ name: "small-1x", vcpu: "1", ram: "512 MB", perSec: "$0.000034", perHour: "$0.122" },
	{ name: "small-2x", vcpu: "2", ram: "1 GB", perSec: "$0.000068", perHour: "$0.245" },
	{ name: "medium-1x", vcpu: "2", ram: "4 GB", perSec: "$0.000130", perHour: "$0.468" },
	{ name: "medium-2x", vcpu: "4", ram: "8 GB", perSec: "$0.000260", perHour: "$0.936" },
	{ name: "large-1x", vcpu: "8", ram: "16 GB", perSec: "$0.000525", perHour: "$1.890" },
	{ name: "large-2x", vcpu: "16", ram: "32 GB", perSec: "$0.001050", perHour: "$3.780" },
];

export function ComputePricingTable() {
	return (
		<section className="py-16 sm:py-20">
			<div className="mx-auto max-w-4xl px-4 sm:px-6 lg:px-8">
				<h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl">
					Compute pricing
				</h2>
				<p className="mt-3 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
					Pay per second of compute time. Choose the preset that fits your workload.
				</p>

				<div className="mt-8 overflow-x-auto rounded-lg border border-border/60">
					<table className="w-full text-left text-sm">
						<thead>
							<tr className="border-b border-border/60 bg-muted/30">
								<th className="px-4 py-3 font-medium text-muted-foreground">Preset</th>
								<th className="px-4 py-3 font-medium text-muted-foreground">vCPU</th>
								<th className="px-4 py-3 font-medium text-muted-foreground">RAM</th>
								<th className="px-4 py-3 font-medium text-muted-foreground text-right">Per Second</th>
								<th className="px-4 py-3 font-medium text-muted-foreground text-right">Per Hour</th>
							</tr>
						</thead>
						<tbody>
							{presets.map((p) => (
								<tr key={p.name} className="border-b border-border/30 last:border-0">
									<td className="px-4 py-3 font-mono text-foreground text-sm">{p.name}</td>
									<td className="px-4 py-3 text-muted-foreground">{p.vcpu}</td>
									<td className="px-4 py-3 text-muted-foreground">{p.ram}</td>
									<td className="px-4 py-3 text-right font-mono text-foreground">{p.perSec}</td>
									<td className="px-4 py-3 text-right font-mono text-foreground">{p.perHour}</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>

				<p className="mt-4 text-muted-foreground/70 text-xs">
					Free tier includes the micro preset with a 10-second max timeout.
					Paid plans include compute credits equal to the subscription price.
				</p>
			</div>
		</section>
	);
}
