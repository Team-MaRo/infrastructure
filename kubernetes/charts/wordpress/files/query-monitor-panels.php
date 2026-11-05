<?php
/**
 * Plugin Name: Query Monitor: S3 and Valkey
 * Description: Adds the requests to the uploads bucket and the calls to Valkey to Query Monitor, and switches its database class on and off with it.
 */

// The marker query-monitor-db.php checks, outside the web root (QUERY_MONITOR_MARKER). A start
// begins without it, as the init container switches Query Monitor off then.
add_action(
	'activated_plugin',
	function ( $plugin ) {
		if ( 'query-monitor/query-monitor.php' === $plugin && defined( 'QUERY_MONITOR_MARKER' ) ) {
			touch( QUERY_MONITOR_MARKER );
		}
	}
);
add_action(
	'deactivated_plugin',
	function ( $plugin ) {
		if ( 'query-monitor/query-monitor.php' === $plugin && defined( 'QUERY_MONITOR_MARKER' ) && file_exists( QUERY_MONITOR_MARKER ) ) {
			unlink( QUERY_MONITOR_MARKER );
		}
	}
);

// S3-Uploads turns file functions on the uploads (file_exists, fopen, …) into requests to the bucket,
// which Query Monitor's HTTP panel does not see. While measuring, a middleware on its AWS client
// records each one. Before S3-Uploads' own setup on plugins_loaded, so the stream wrapper gets this
// client; the first requests come from plugins setting themselves up.
if ( defined( 'QUERY_MONITOR_MEASURING' ) ) {
	$GLOBALS['query_monitor_s3_calls'] = array();

	add_action(
		'plugins_loaded',
		function () {
			if ( ! class_exists( 'S3_Uploads\Plugin' ) || ! defined( 'S3_UPLOADS_BUCKET' ) ) {
				return;
			}
			S3_Uploads\Plugin::get_instance()->s3()->getHandlerList()->appendSign(
				function ( callable $handler ) {
					return function ( Aws\CommandInterface $command, $request = null ) use ( $handler ) {
						$start  = microtime( true );
						$caller = query_monitor_s3_caller();
						return $handler( $command, $request )->then(
							function ( $result ) use ( $command, $start, $caller ) {
								query_monitor_s3_record( $command, $start, $caller, '' );
								return $result;
							},
							function ( $reason ) use ( $command, $start, $caller ) {
								query_monitor_s3_record( $command, $start, $caller, $reason instanceof Throwable ? $reason->getMessage() : 'failed' );
								return GuzzleHttp\Promise\Create::rejectionFor( $reason );
							}
						);
					};
				},
				'query-monitor'
			);
		},
		PHP_INT_MIN
	);
}

/**
 * The plugin or theme whose code led to the request, with the function it called there.
 */
function query_monitor_s3_caller(): string {
	foreach ( debug_backtrace( DEBUG_BACKTRACE_IGNORE_ARGS ) as $frame ) {
		if ( empty( $frame['file'] ) || ! preg_match( '#/wp-content/(?:plugins|themes)/([^/]+)/#', $frame['file'], $match ) || 's3-uploads' === $match[1] ) {
			continue;
		}
		return sprintf( '%s: %s() in %s:%d', $match[1], $frame['function'] ?? '?', basename( $frame['file'] ), $frame['line'] ?? 0 );
	}
	return 'WordPress';
}

function query_monitor_s3_record( Aws\CommandInterface $command, float $start, string $caller, string $error ): void {
	$GLOBALS['query_monitor_s3_calls'][] = array(
		'operation' => $command->getName(),
		'key'       => (string) ( $command['Key'] ?? $command['Prefix'] ?? '' ),
		'ms'        => ( microtime( true ) - $start ) * 1000,
		'caller'    => $caller,
		'error'     => $error,
	);
}

// The panels exist only where Query Monitor runs, so its classes are there.
add_filter(
	'qm/collectors',
	function ( array $collectors ) {
		$collectors['s3'] = new class() extends QM_DataCollector {
			public $id = 's3';

			public function get_storage(): QM_Data {
				return new class() extends QM_Data {};
			}

			public function process() {
				$this->data->calls = $GLOBALS['query_monitor_s3_calls'] ?? null;
			}
		};

		// Redis Object Cache counts its round trips to Valkey and their time per request, but its
		// own panel shows neither.
		$collectors['valkey'] = new class() extends QM_DataCollector {
			public $id = 'valkey';

			public function get_storage(): QM_Data {
				return new class() extends QM_Data {};
			}

			public function process() {
				global $wp_object_cache;
				$this->data->info = is_object( $wp_object_cache ) && method_exists( $wp_object_cache, 'info' ) ? $wp_object_cache->info() : null;
			}
		};

		return $collectors;
	}
);

add_filter(
	'qm/outputter/html',
	function ( array $output ) {
		$s3 = QM_Collectors::get( 's3' );
		if ( $s3 ) {
			$output['s3'] = new class( $s3 ) extends QM_Output_Html {
				public function __construct( QM_Collector $collector ) {
					parent::__construct( $collector );
					add_filter( 'qm/output/menus', array( $this, 'admin_menu' ), 101 );
				}

				public function name() {
					return 'S3';
				}

				public function admin_menu( array $menu ) {
					$calls = $this->collector->get_data()->calls;
					$title = null === $calls ? 'S3' : sprintf( 'S3 (%d, %.0f ms)', count( $calls ), array_sum( array_column( $calls, 'ms' ) ) );
					$menu[ $this->collector->id() ] = $this->menu( array( 'title' => $title ) );
					return $menu;
				}

				public function output() {
					$calls = $this->collector->get_data()->calls;
					if ( null === $calls ) {
						$this->before_non_tabular_output();
						echo $this->build_notice( 'Recorded only for a logged-in request while Query Monitor is active.' ); // phpcs:ignore WordPress.Security.EscapeOutput.OutputNotEscaped
						$this->after_non_tabular_output();
						return;
					}
					$this->before_tabular_output();
					echo '<thead><tr><th scope="col">Operation</th><th scope="col">Key</th><th scope="col">Caller</th><th scope="col">Error</th><th scope="col" class="qm-num">Time (ms)</th></tr></thead><tbody>';
					foreach ( $calls as $call ) {
						printf(
							'<tr><td class="qm-ltr">%s</td><td class="qm-ltr">%s</td><td class="qm-ltr">%s</td><td>%s</td><td class="qm-num">%.1f</td></tr>',
							esc_html( $call['operation'] ),
							esc_html( $call['key'] ),
							esc_html( $call['caller'] ),
							esc_html( $call['error'] ),
							$call['ms']
						);
					}
					printf(
						'</tbody><tfoot><tr><td colspan="4">Total: %d</td><td class="qm-num">%.1f</td></tr></tfoot>',
						count( $calls ),
						array_sum( array_column( $calls, 'ms' ) )
					);
					$this->after_tabular_output();
				}
			};
		}

		$valkey = QM_Collectors::get( 'valkey' );
		if ( $valkey ) {
			$output['valkey'] = new class( $valkey ) extends QM_Output_Html {
				public function __construct( QM_Collector $collector ) {
					parent::__construct( $collector );
					add_filter( 'qm/output/menus', array( $this, 'admin_menu' ), 102 );
				}

				public function name() {
					return 'Valkey';
				}

				public function admin_menu( array $menu ) {
					$info  = $this->collector->get_data()->info;
					$title = null === $info ? 'Valkey' : sprintf( 'Valkey (%d, %.0f ms)', $info->calls, $info->time * 1000 );
					$menu[ $this->collector->id() ] = $this->menu( array( 'title' => $title ) );
					return $menu;
				}

				public function output() {
					$info = $this->collector->get_data()->info;
					$this->before_non_tabular_output();
					if ( null === $info ) {
						echo $this->build_notice( 'No Redis Object Cache drop-in on this site.' ); // phpcs:ignore WordPress.Security.EscapeOutput.OutputNotEscaped
					} else {
						foreach (
							array(
								'Calls to Valkey' => (string) $info->calls,
								'Time in Valkey'  => sprintf( '%.1f ms', $info->time * 1000 ),
								'Lookups'         => sprintf( '%d (%d hits, %d misses)', $info->hits + $info->misses, $info->hits, $info->misses ),
							) as $label => $value
						) {
							printf( '<section><h3>%s</h3><p class="qm-ltr"><code>%s</code></p></section>', esc_html( $label ), esc_html( $value ) );
						}
					}
					$this->after_non_tabular_output();
				}
			};
		}

		return $output;
	},
	110
);
