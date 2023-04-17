use std::io::{self, Stdout};

use anyhow::Result;
use crossterm::{
    event::{self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use tui::{
    backend::{Backend, CrosstermBackend},
    layout::{Constraint, Layout},
    style::{Color, Modifier, Style},
    widgets::{Block, Borders, Cell, Row, Table, TableState},
    Frame, Terminal as TuiTerminal,
};
use venus_worker_util::sealcalc;

/// Display the status of tasks running at different times by tui's table
pub fn display(
    items: &[sealcalc::Item],
    tree_d_concurrent: usize,
    pc1_concurrent: usize,
    pc2_concurrent: usize,
    c2_concurrent: usize,
    sealing_threads: usize,
) -> Result<()> {
    App {
        items,
        tree_d_concurrent,
        pc1_concurrent,
        pc2_concurrent,
        c2_concurrent,
        sealing_threads,
        state: Default::default(),
    }
    .run()
}

struct App<'a> {
    items: &'a [sealcalc::Item],
    tree_d_concurrent: usize,
    pc1_concurrent: usize,
    pc2_concurrent: usize,
    c2_concurrent: usize,
    sealing_threads: usize,

    state: TableState,
}

impl<'a> App<'a> {
    pub fn run(&mut self) -> Result<()> {
        let mut terminal = setup_terminal();
        // select first row
        self.state.select(Some(0));
        loop {
            terminal.inner_mut().draw(|f| self.ui(f))?;

            if let Event::Key(key) = event::read()? {
                match key.code {
                    KeyCode::Char('q') => return Ok(()),
                    KeyCode::Down => self.next(1),
                    KeyCode::Up => self.previous(1),
                    KeyCode::Left => self.previous(20),
                    KeyCode::Char(' ') | KeyCode::Right => self.next(20),
                    _ => {}
                }
            }
        }
    }

    fn previous(&mut self, size: usize) {
        let i = match self.state.selected() {
            Some(i) => {
                if i == 0 {
                    self.items.len() - 1
                } else if i < size {
                    0
                } else {
                    i - size
                }
            }
            None => 0,
        };
        self.state.select(Some(i));
    }

    fn next(&mut self, size: usize) {
        let i = match self.state.selected() {
            Some(i) => {
                let len = self.items.len();
                if i == len - 1 {
                    0
                } else if i >= len - size {
                    len - 1
                } else {
                    i + size
                }
            }
            None => 0,
        };
        self.state.select(Some(i));
    }

    fn ui<B: Backend>(&mut self, f: &mut Frame<B>) {
        let rects = Layout::default()
            .constraints([Constraint::Percentage(100)].as_ref())
            .split(f.size());

        let selected_style = Style::default().add_modifier(Modifier::REVERSED);
        let normal_style = Style::default().bg(Color::Blue);
        let header_cells = [
            "time\n(mins)",
            "sealing threads\n(running/total)",
            "  tree_d\n(running/total)",
            "    pc1\n(running/total)",
            "    pc2\n(running/total)",
            "wait\nseed",
            "    c2\n(running/total)",
            "finished\nsectors",
        ]
        .iter()
        .map(|h| Cell::from(*h).style(Style::default().fg(Color::LightYellow)));

        let header = Row::new(header_cells)
            .style(normal_style)
            .height(2)
            .bottom_margin(1);

        let rows = self.items.iter().map(|item| {
            Row::new([
                Cell::from(item.time_in_mins.to_string()),
                Cell::from(format!(
                    "{}/{}",
                    item.sealing_threads_running, self.sealing_threads
                )),
                Cell::from(format!(
                    "{}/{}",
                    item.tree_d_running, self.tree_d_concurrent
                )),
                Cell::from(format!("{}/{}", item.pc1_running, self.pc1_concurrent)),
                Cell::from(format!("{}/{}", item.pc2_running, self.pc2_concurrent)),
                Cell::from(item.seed_waiting.to_string()),
                Cell::from(format!("{}/{}", item.c2_running, self.c2_concurrent)),
                Cell::from(item.finished_sectors.to_string()),
            ])
        });
        let widths = [
            [Constraint::Ratio(1, 14)].as_slice(),
            [Constraint::Ratio(1, 7); 4].as_slice(),
            [Constraint::Ratio(1, 14)].as_slice(),
            [Constraint::Ratio(1, 7); 2].as_slice(),
        ]
        .concat();

        let t = Table::new(rows)
            .header(header)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title("sealing calculator"),
            )
            .highlight_style(selected_style)
            .widths(widths.as_slice());
        f.render_stateful_widget(t, rects[0], &mut self.state);
    }
}

struct Terminal(TuiTerminal<CrosstermBackend<Stdout>>);

impl Terminal {
    fn inner_mut(&mut self) -> &mut TuiTerminal<CrosstermBackend<Stdout>> {
        &mut self.0
    }
}

impl Drop for Terminal {
    fn drop(&mut self) {
        // cleanup terminal
        disable_raw_mode().expect("Unable to disable raw mode.");
        execute!(
            self.0.backend_mut(),
            LeaveAlternateScreen,
            DisableMouseCapture
        )
        .expect("Unable to leave alternate screen.");
        self.0.show_cursor().expect("Unable to show cursor");
    }
}

fn setup_terminal() -> Terminal {
    enable_raw_mode().expect("Unable to enter raw mode.");
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture)
        .expect("Unable to enter alternate screen");
    let backend = CrosstermBackend::new(stdout);
    let tui_terminal =
        TuiTerminal::new(backend).expect("Couldn't create new terminal with backend");

    Terminal(tui_terminal)
}
